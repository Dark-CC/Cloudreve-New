package oneyunpan

import (
	"context"
	"testing"

	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
)

// A 1yunpan policy may only be relayed through this node when no usable edge is
// configured: relaying pushes the whole file in one request, which large files
// cannot survive behind a reverse proxy.
func TestEffectiveRelayFollowsEdgeConfig(t *testing.T) {
	key := "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="

	cases := []struct {
		name    string
		policy  *ent.StoragePolicy
		relayed bool
	}{
		{
			name:    "no edge configured",
			policy:  &ent.StoragePolicy{Type: types.PolicyTypeOneyunpan, Settings: &types.PolicySetting{}},
			relayed: true,
		},
		{
			name: "legacy policy copy with edge and key",
			policy: &ent.StoragePolicy{
				Type: types.PolicyTypeOneyunpan,
				Settings: &types.PolicySetting{
					OneyunpanEdgeEndpoint: "https://upload.example.com",
					OneyunpanTicketKey:    key,
				},
			},
			relayed: false,
		},
		{
			name: "edge without key",
			policy: &ent.StoragePolicy{
				Type: types.PolicyTypeOneyunpan,
				Settings: &types.PolicySetting{
					OneyunpanEdgeEndpoint: "https://upload.example.com",
				},
			},
			relayed: true,
		},
		{
			name: "edge with malformed key",
			policy: &ent.StoragePolicy{
				Type: types.PolicyTypeOneyunpan,
				Settings: &types.PolicySetting{
					OneyunpanEdgeEndpoint: "https://upload.example.com",
					OneyunpanTicketKey:    "not-a-32-byte-key",
				},
			},
			relayed: true,
		},
		{
			name:   "nil policy",
			policy: nil,

			relayed: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := EffectiveRelay(context.Background(), nil, tc.policy); got != tc.relayed {
				t.Fatalf("EffectiveRelay() = %t, want %t", got, tc.relayed)
			}
		})
	}
}

