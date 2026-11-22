package inventory

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/setting"
	"github.com/cloudreve/Cloudreve/v4/ent/storagepolicy"
	"github.com/cloudreve/Cloudreve/v4/inventory/debug"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/cache"
	"github.com/cloudreve/Cloudreve/v4/pkg/logging"
)

// Site-level 1yunpan edge upload settings. They used to live on every storage
// policy; keeping them here lets operators configure the shared edge once while
// the policy form only asks for the 1yunpan account.
const (
	OneyunpanEdgeEndpointSetting  = "oneyunpan_edge_endpoint"
	OneyunpanEdgeTicketKeySetting = "oneyunpan_edge_ticket_key"
	OneyunpanEdgeSiteSetting      = "oneyunpan_edge_site"
)

// EnsureOneyunpanEdgeSettings seeds the site-level edge configuration from an
// existing 1yunpan policy. Upgrades therefore keep working without the operator
// re-entering the ticket key, and a fresh installation simply gets empty values.
func EnsureOneyunpanEdgeSettings(ctx context.Context, client *ent.Client, kv cache.Driver, l logging.Logger) error {
	existing, err := client.Setting.Query().
		Where(setting.NameIn(
			OneyunpanEdgeEndpointSetting,
			OneyunpanEdgeTicketKeySetting,
			OneyunpanEdgeSiteSetting,
		)).
		All(ctx)
	if err != nil {
		return fmt.Errorf("failed to query 1yunpan edge settings: %w", err)
	}

	present := make(map[string]string, len(existing))
	for _, s := range existing {
		present[s.Name] = s.Value
	}

	// Backfill from an existing policy first. This must happen before the relay
	// reconciliation below, which reads the merged view.
	if err := seedOneyunpanEdgeSettings(ctx, client, kv, l, present); err != nil {
		return err
	}

	return ReconcileOneyunpanEdgeRelay(ctx, client, kv, l)
}

// seedOneyunpanEdgeSettings copies the legacy per-policy edge configuration
// into the site-wide settings. Any field that is already present is left
// untouched.
func seedOneyunpanEdgeSettings(
	ctx context.Context,
	client *ent.Client,
	kv cache.Driver,
	l logging.Logger,
	present map[string]string,
) error {
	// Only look at policies when something still needs to be seeded.
	var seed *types.PolicySetting
	if strings.TrimSpace(present[OneyunpanEdgeEndpointSetting]) == "" {
		policies, err := client.StoragePolicy.Query().
			Where(storagepolicy.TypeEQ(string(types.PolicyTypeOneyunpan))).
			Order(ent.Asc(storagepolicy.FieldID)).
			All(ctx)
		if err != nil {
			return fmt.Errorf("failed to query 1yunpan storage policies: %w", err)
		}
		for _, policy := range policies {
			if policy.Settings == nil || strings.TrimSpace(policy.Settings.OneyunpanEdgeEndpoint) == "" {
				continue
			}
			seed = policy.Settings
			break
		}
	}

	updates := map[string]string{}
	if seed != nil {
		if strings.TrimSpace(present[OneyunpanEdgeEndpointSetting]) == "" {
			updates[OneyunpanEdgeEndpointSetting] = seed.OneyunpanEdgeEndpoint
		}
		if strings.TrimSpace(present[OneyunpanEdgeTicketKeySetting]) == "" {
			updates[OneyunpanEdgeTicketKeySetting] = seed.OneyunpanTicketKey
		}
		if strings.TrimSpace(present[OneyunpanEdgeSiteSetting]) == "" {
			updates[OneyunpanEdgeSiteSetting] = seed.OneyunpanEdgeSite
		}
	}

	if len(updates) == 0 {
		return nil
	}

	ctx = context.WithValue(ctx, debug.SkipDbLogging{}, true)
	for name, value := range updates {
		if _, ok := present[name]; ok {
			if err := client.Setting.Update().
				Where(setting.Name(name)).
				SetValue(value).
				Exec(ctx); err != nil {
				return fmt.Errorf("failed to seed 1yunpan edge setting %q: %w", name, err)
			}
			continue
		}
		if _, err := client.Setting.Create().SetName(name).SetValue(value).Save(ctx); err != nil {
			return fmt.Errorf("failed to create 1yunpan edge setting %q: %w", name, err)
		}
	}

	if kv != nil {
		if err := kv.Delete(kvSettingPrefix); err != nil && l != nil {
			l.Warning("Failed to clear setting cache after seeding 1yunpan edge settings: %s", err)
		}
	}
	if l != nil {
		l.Info("Seeded %d 1yunpan edge setting(s) from the existing storage policy.", len(updates))
	}

	return nil
}

// ReconcileOneyunpanEdgeRelay aligns the relay flag stored on every 1yunpan
// policy with the site-wide edge configuration. The flag is only a cached
// projection: uploads can only be relayed through this node when no edge is
// configured, and relaying a large file fails against the reverse proxy body
// limit. Fixing it at boot makes stale rows self-heal after an upgrade instead
// of requiring the operator to re-save each policy.
func ReconcileOneyunpanEdgeRelay(ctx context.Context, client *ent.Client, kv cache.Driver, l logging.Logger) error {
	edgeEnabled := strings.TrimSpace(getOneyunpanEdgeValue(ctx, client, OneyunpanEdgeEndpointSetting)) != ""

	policies, err := client.StoragePolicy.Query().
		Where(storagepolicy.TypeEQ(string(types.PolicyTypeOneyunpan))).
		All(ctx)
	if err != nil {
		return fmt.Errorf("failed to query 1yunpan policies for relay reconciliation: %w", err)
	}

	fixed := 0
	for _, policy := range policies {
		if policy.Settings == nil {
			policy.Settings = &types.PolicySetting{}
		}
		legacyCopy := policy.Settings.OneyunpanEdgeEndpoint != "" ||
			policy.Settings.OneyunpanTicketKey != "" ||
			policy.Settings.OneyunpanEdgeSite != ""

		// Without an edge endpoint the driver can only stream through this node,
		// so relay has to stay on. With one configured, relaying would push the
		// whole file through this node and hit the reverse proxy body limit.
		wantRelay := !edgeEnabled
		if policy.Settings.Relay == wantRelay && !(edgeEnabled && legacyCopy) {
			continue
		}

		if edgeEnabled {
			// The legacy per-policy copy has been seeded into the site settings,
			// so it can be dropped to keep the ticket key in exactly one place.
			policy.Settings.OneyunpanEdgeEndpoint = ""
			policy.Settings.OneyunpanTicketKey = ""
			policy.Settings.OneyunpanEdgeSite = ""
		}
		policy.Settings.Relay = wantRelay
		if _, err := client.StoragePolicy.UpdateOne(policy).SetSettings(policy.Settings).Save(ctx); err != nil {
			return fmt.Errorf("failed to update relay flag for 1yunpan policy %d: %w", policy.ID, err)
		}
		if kv != nil {
			if err := kv.Delete(StoragePolicyCacheKey, strconv.Itoa(policy.ID)); err != nil && l != nil {
				l.Warning("Failed to clear storage policy cache for policy %d: %s", policy.ID, err)
			}
		}
		fixed++
	}

	if fixed > 0 && l != nil {
		l.Info("Aligned %d 1yunpan policy(ies); edge upload endpoint configured: %t.", fixed, edgeEnabled)
	}
	return nil
}

// getOneyunpanEdgeValue reads a setting written in this boot cycle. The values
// were just queried or inserted, so a direct read avoids relying on the cache
// that was invalidated a moment ago.
func getOneyunpanEdgeValue(ctx context.Context, client *ent.Client, name string) string {
	row, err := client.Setting.Query().Where(setting.Name(name)).Only(ctx)
	if err != nil {
		return ""
	}
	return row.Value
}
