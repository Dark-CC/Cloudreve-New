package share

import (
	"context"

	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/user"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/hashid"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/gin-gonic/gin"
)

type (
	// ShareACLService reads and replaces the fine grained permissions of a share.
	ShareACLService struct {
		ID  string              `uri:"id" binding:"required"`
		ACL *inventory.ShareACL `json:"acl"`
	}
	ShareACLParamCtx struct{}

	// ShareACLResponse is the ACL view of a share.
	ShareACLResponse struct {
		Builtin []inventory.BuiltinSharePermission `json:"builtin"`
		Group   []inventory.GroupSharePermission   `json:"group"`
		User    []inventory.UserSharePermission    `json:"user"`
	}
)

// Get returns the ACL of a share owned by the requester.
func (s *ShareACLService) Get(c *gin.Context) (*ShareACLResponse, error) {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)

	shareID, err := dep.HashIDEncoder().Decode(s.ID, hashid.ShareID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeParamErr, "Invalid share ID", err)
	}

	if _, err := dep.ShareClient().GetByIDUser(c, shareID, user.ID); err != nil {
		return nil, serializer.NewError(serializer.CodeNotFound, "Share not found", err)
	}

	acl, err := dep.ShareACLClient().List(c, shareID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to get share permissions", err)
	}

	return newShareACLResponse(acl, dep.HashIDEncoder()), nil
}

// Update replaces the ACL of a share owned by the requester.
func (s *ShareACLService) Update(c *gin.Context) (*ShareACLResponse, error) {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)

	shareID, err := dep.HashIDEncoder().Decode(s.ID, hashid.ShareID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeParamErr, "Invalid share ID", err)
	}

	if _, err := dep.ShareClient().GetByIDUser(c, shareID, user.ID); err != nil {
		return nil, serializer.NewError(serializer.CodeNotFound, "Share not found", err)
	}

	if err := validateACL(c, user, s.ACL); err != nil {
		return nil, err
	}

	aclClient, tx, txCtx, err := inventory.WithTx(c, dep.ShareACLClient())
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to start permission transaction", err)
	}
	defer func() {
		_ = inventory.Rollback(tx)
	}()

	if err := aclClient.Replace(txCtx, shareID, s.ACL); err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to update share permissions", err)
	}
	if err := inventory.Commit(tx); err != nil {
		dependency.FromContext(c).Logger().Warning("Failed to commit share ACL for share %d: %s", shareID, err)
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to commit share permissions", err)
	}

	acl, err := dep.ShareACLClient().List(c, shareID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to read share permissions", err)
	}

	return newShareACLResponse(acl, dep.HashIDEncoder()), nil
}

func validateACL(ctx context.Context, owner *ent.User, acl *inventory.ShareACL) error {
	if acl == nil {
		return nil
	}

	validAudience := map[string]bool{
		inventory.ShareAudienceAnonymous:  true,
		inventory.ShareAudienceLoggedIn:   true,
		inventory.ShareAudienceSameGroup:  true,
		inventory.ShareAudienceOtherGroup: true,
	}

	seen := make(map[string]bool)
	for _, item := range acl.Builtin {
		if !validAudience[item.Audience] {
			return serializer.NewError(serializer.CodeParamErr, "Unknown share audience", nil)
		}
		if seen[item.Audience] {
			return serializer.NewError(serializer.CodeParamErr, "Duplicate share audience", nil)
		}
		if err := validateSharePermissionCeiling(owner, true, item.Permission); err != nil {
			return err
		}
		seen[item.Audience] = true
	}

	seenGroups := make(map[int]bool)
	dep := dependency.FromContext(ctx)
	for _, item := range acl.Group {
		if item.GroupID <= 0 {
			return serializer.NewError(serializer.CodeParamErr, "Invalid group", nil)
		}
		if item.GroupID == inventory.AnonymousGroupID {
			return serializer.NewError(serializer.CodeParamErr, "Anonymous group cannot be used as an explicit rule", nil)
		}
		if seenGroups[item.GroupID] {
			return serializer.NewError(serializer.CodeParamErr, "Duplicate group permission", nil)
		}
		if _, err := dep.GroupClient().GetByID(ctx, item.GroupID); err != nil {
			if ent.IsNotFound(err) {
				return serializer.NewError(serializer.CodeParamErr, "Group not found", err)
			}
			return serializer.NewError(serializer.CodeDBError, "Failed to validate group", err)
		}
		if err := validateSharePermissionCeiling(owner, false, item.Permission); err != nil {
			return err
		}
		seenGroups[item.GroupID] = true
	}

	seenUsers := make(map[int]bool)
	for i := range acl.User {
		item := &acl.User[i]
		if item.UserHashID != "" {
			decodedUserID, decodeErr := dep.HashIDEncoder().Decode(item.UserHashID, hashid.UserID)
			if decodeErr != nil {
				return serializer.NewError(serializer.CodeParamErr, "Invalid user", decodeErr)
			}
			if item.UserID > 0 && item.UserID != decodedUserID {
				return serializer.NewError(serializer.CodeParamErr, "User ID does not match user hash ID", nil)
			}
			item.UserID = decodedUserID
		}
		if item.UserID <= 0 {
			return serializer.NewError(serializer.CodeParamErr, "Invalid user", nil)
		}
		if seenUsers[item.UserID] {
			return serializer.NewError(serializer.CodeParamErr, "Duplicate user permission", nil)
		}
		target, err := dep.UserClient().GetByID(ctx, item.UserID)
		if err != nil {
			if ent.IsNotFound(err) {
				return serializer.NewError(serializer.CodeParamErr, "User not found", err)
			}
			return serializer.NewError(serializer.CodeDBError, "Failed to validate user", err)
		}
		if target.Status != user.StatusActive {
			return serializer.NewError(serializer.CodeParamErr, "User is not active", nil)
		}
		if err := validateSharePermissionCeiling(owner, false, item.Permission); err != nil {
			return err
		}
		seenUsers[item.UserID] = true
	}

	return nil
}

func newShareACLResponse(acl *inventory.ShareACL, encoder hashid.Encoder) *ShareACLResponse {
	res := &ShareACLResponse{}
	if acl == nil {
		return res
	}

	res.Builtin = acl.Builtin
	res.Group = acl.Group
	res.User = make([]inventory.UserSharePermission, len(acl.User))
	copy(res.User, acl.User)
	for i := range res.User {
		res.User[i].UserHashID = hashid.EncodeUserID(encoder, res.User[i].UserID)
	}
	return res
}

// validateSharePermissionCeiling mirrors file level permission validation:
// granting write capabilities through a builtin audience or an explicit
// user/group rule requires the owner's group to allow that class of grants.
func validateSharePermissionCeiling(owner *ent.User, builtin bool, permission inventory.SharePermission) error {
	if !permission.CanUpdate && !permission.CanCreate && !permission.CanDelete && !permission.CanShare {
		return nil
	}
	if owner == nil || owner.Edges.Group == nil {
		return serializer.NewError(serializer.CodeParamErr, "Owner group is not loaded", nil)
	}

	permissionBit := types.GroupPermissionSetExplicitUser
	if builtin {
		permissionBit = types.GroupPermissionSetAnonymousPermission
	}
	if !owner.Edges.Group.Permissions.Enabled(int(permissionBit)) {
		return serializer.NewError(serializer.CodeParamErr,
			"Your group cannot grant write permissions in this share permission rule", nil)
	}
	return nil
}
