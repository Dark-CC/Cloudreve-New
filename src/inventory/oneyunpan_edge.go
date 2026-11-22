package inventory

import (
	"context"
	"fmt"
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
