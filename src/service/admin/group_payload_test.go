package admin

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func decodeGroupPayload(t *testing.T, raw string) *UpsertGroupService {
	t.Helper()

	var service UpsertGroupService
	require.NoError(t, json.Unmarshal([]byte(raw), &service))
	return &service
}

// Pro's admin UI round-trips edges.storage_policies as an array. Binding that
// straight into ent.Group failed with a JSON type error, which surfaced as
// "JSON marshall error" when saving a group.
func TestUpsertGroupAcceptsPolicyArray(t *testing.T) {
	service := decodeGroupPayload(t, `{
		"group": {
			"id": 2,
			"name": "User-CN",
			"max_storage": 10737418240,
			"speed_limit": 0,
			"permissions": "jJgEQA==",
			"settings": {},
			"edges": {"storage_policies": [{"id": 2}, {"id": 5}]}
		},
		"storage_policy_ids": [2, 5]
	}`)

	group, err := service.Group.toEnt()
	require.NoError(t, err)
	require.Equal(t, 2, group.ID)
	require.Equal(t, "User-CN", group.Name)
	require.Equal(t, int64(10737418240), group.MaxStorage)
	require.NotNil(t, group.Permissions)
	require.Equal(t, 2, group.StoragePolicyID)
	require.NotNil(t, group.Edges.StoragePolicies)
	require.Equal(t, 2, group.Edges.StoragePolicies.ID)
}

// The community frontend sends a single object, which must keep working.
func TestUpsertGroupAcceptsPolicyObject(t *testing.T) {
	service := decodeGroupPayload(t, `{
		"group": {
			"id": 4,
			"name": "User-HK",
			"permissions": "jBg=",
			"edges": {"storage_policies": {"id": 5}}
		}
	}`)

	group, err := service.Group.toEnt()
	require.NoError(t, err)
	require.Equal(t, 5, group.StoragePolicyID)
	require.Equal(t, 5, group.Edges.StoragePolicies.ID)
}

func TestUpsertGroupWithoutPolicyEdge(t *testing.T) {
	service := decodeGroupPayload(t, `{"group": {"id": 3, "name": "Anonymous", "permissions": "gg=="}}`)

	group, err := service.Group.toEnt()
	require.NoError(t, err)
	require.Equal(t, 0, group.StoragePolicyID)
	require.Nil(t, group.Edges.StoragePolicies)
}

func TestUpsertGroupRejectsMalformedPolicyEdge(t *testing.T) {
	service := decodeGroupPayload(t, `{"group": {"id": 8, "name": "x", "edges": {"storage_policies": "bad"}}}`)

	_, err := service.Group.toEnt()
	require.Error(t, err)
}
