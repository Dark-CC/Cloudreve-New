package oneyunpan

import (
	"context"
	"strings"

	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/pkg/setting"
)

// EdgeConfig is the resolved edge upload configuration for one operation.
type EdgeConfig struct {
	Endpoint  string
	TicketKey string
	Site      string
}

// Enabled reports whether browser uploads should be handed to the edge.
func (c EdgeConfig) Enabled() bool {
	return c.Endpoint != ""
}

// EffectiveRelay reports whether a browser upload for this policy has to be
// relayed through this node. 1yunpan browser uploads only work through the
// edge: its Put implementation cannot accept partial chunks, so a relayed
// upload has to push the whole file in a single request and trips reverse
// proxy body limits on anything but small files. The relay flag stored on the
// policy is therefore just a projection of the site-wide edge settings and
// must never be trusted on its own, otherwise an install that configured the
// edge after creating the policy keeps relaying and fails on large files.
func EffectiveRelay(ctx context.Context, provider setting.Provider, policy *ent.StoragePolicy) bool {
	edge := ResolveEdgeConfig(ctx, provider, policy)
	if !edge.Enabled() {
		return true
	}
	// An endpoint without a usable ticket key cannot seal a ticket, so the edge
	// route would fail on the first upload. Treat that half-configured state as
	// "no edge" and keep relaying instead of breaking every upload.
	if _, err := DecodeTicketKey(edge.TicketKey); err != nil {
		return true
	}
	return false
}

// ResolveEdgeConfig reads the deployment-wide edge settings and falls back to
// the values stored on the policy itself, so installations upgraded from the
// old per-policy layout keep working before the site settings are seeded.
func ResolveEdgeConfig(ctx context.Context, provider setting.Provider, policy *ent.StoragePolicy) EdgeConfig {
	var cfg EdgeConfig
	if provider != nil {
		site := provider.OneyunpanEdge(ctx)
		cfg = EdgeConfig{
			Endpoint:  strings.TrimSpace(site.Endpoint),
			TicketKey: strings.TrimSpace(site.TicketKey),
			Site:      strings.TrimSpace(site.Site),
		}
	}

	if policy == nil || policy.Settings == nil {
		return cfg
	}

	legacy := policy.Settings
	if cfg.Endpoint == "" {
		cfg.Endpoint = strings.TrimSpace(legacy.OneyunpanEdgeEndpoint)
	}
	if cfg.TicketKey == "" {
		cfg.TicketKey = strings.TrimSpace(legacy.OneyunpanTicketKey)
	}
	if cfg.Site == "" {
		cfg.Site = strings.TrimSpace(legacy.OneyunpanEdgeSite)
	}

	return cfg
}
