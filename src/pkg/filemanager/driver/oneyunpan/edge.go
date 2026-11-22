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
