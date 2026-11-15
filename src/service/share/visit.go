package share

import (
	"context"
	"strings"

	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/cluster/routes"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/fs"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/manager"
	"github.com/cloudreve/Cloudreve/v4/pkg/hashid"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/cloudreve/Cloudreve/v4/service/audit"
	"github.com/cloudreve/Cloudreve/v4/service/explorer"
	"github.com/gin-gonic/gin"
)

type (
	ShortLinkRedirectService struct {
		ID       string `uri:"id" binding:"required"`
		Password string `uri:"password"`
	}
	ShortLinkRedirectParamCtx struct{}
)

func (s *ShortLinkRedirectService) RedirectTo(c *gin.Context) string {
	shareLongUrl := routes.MasterShareLongUrl(s.ID, s.Password)

	shortLinkQuery := c.Request.URL.Query() // Query in ShortLink, adapt to Cloudreve V3
	shareLongUrlQuery := shareLongUrl.Query()

	userSpecifiedPath := shortLinkQuery.Get("path")
	if userSpecifiedPath != "" {
		masterPath := shareLongUrlQuery.Get("path")
		masterPath += "/" + strings.TrimPrefix(userSpecifiedPath, "/")

		shareLongUrlQuery.Set("path", masterPath)
	}

	shortLinkQuery.Del("path") // 防止用户指定的 Path 就是空字符串
	for k, vals := range shortLinkQuery {
		shareLongUrlQuery[k] = append(shareLongUrlQuery[k], vals...)
	}

	shareLongUrl.RawQuery = shareLongUrlQuery.Encode()
	return shareLongUrl.String()
}

type (
	ShareInfoService struct {
		Password      string `form:"password"`
		CountViews    bool   `form:"count_views"`
		OwnerExtended bool   `form:"owner_extended"`
	}
	ShareInfoParamCtx struct{}
)

func (s *ShareInfoService) Get(c *gin.Context) (*explorer.Share, error) {
	dep := dependency.FromContext(c)
	u := inventory.UserFromContext(c)
	shareClient := dep.ShareClient()

	ctx := context.WithValue(c, inventory.LoadShareUser{}, true)
	ctx = context.WithValue(ctx, inventory.LoadUserGroup{}, true)
	ctx = context.WithValue(ctx, inventory.LoadShareFile{}, true)
	share, err := shareClient.GetByID(ctx, hashid.FromContext(c))
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, serializer.NewError(serializer.CodeNotFound, "Share not found", nil)
		}
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to get share", err)
	}

	if err := inventory.IsValidShare(share); err != nil {
		return nil, serializer.NewError(serializer.CodeNotFound, "Share link expired", err)
	}

	// ACL is evaluated before the share payload is built, otherwise an
	// audience without read/list permission could still learn the shared file
	// name and size from the basic share response.
	permission := effectiveSharePermission(ctx, dep, u, share)
	hasListPermission := permission.CanList
	hasReadPermission := permission.CanRead

	if s.CountViews && hasListPermission {
		_ = shareClient.Viewed(c, share)
		entry := &audit.LogEntry{
			Name: share.Edges.File.Name,
			From: fs.NewShareUri(hashid.EncodeShareID(dep.HashIDEncoder(), share.ID), ""),
		}
		audit.Record(c, audit.EventShareLinkViewed, entry,
			audit.WithShare(share.ID), audit.WithFile(share.Edges.File.ID))
	}

	unlocked := true
	// Share requires password
	if share.Password != "" && s.Password != share.Password && share.Edges.User.ID != u.ID {
		unlocked = false
	}

	base := dep.SettingProvider().SiteURL(c)
	shareName := share.Edges.File.Name
	if !hasListPermission {
		shareName = ""
	}
	res := explorer.BuildShare(c, share, base, dep.HashIDEncoder(), u, share.Edges.User, shareName,
		types.FileType(share.Edges.File.Type), unlocked, false)
	if !hasListPermission {
		res.RemainDownloads = nil
		res.Downloaded = 0
		res.Size = 0
	}
	if !hasReadPermission {
		res.SourceType = nil
	}
	res.Price, err = shareClient.GetPrice(c, share.ID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to get share price", err)
	}
	if res.Price > 0 && !inventory.IsAnonymousUser(u) && share.Edges.User.ID != u.ID {
		res.Purchased, err = dep.VASClient().HasSharePurchase(c, share.ID, u.ID)
		if err != nil {
			return nil, serializer.NewError(serializer.CodeDBError, "Failed to check share purchase", err)
		}
	}

	if s.OwnerExtended && share.Edges.User.ID == u.ID {
		// Add more information about the shared file
		m := manager.NewFileManager(dep, u)
		defer m.Recycle()

		shareUri, err := fs.NewUriFromString(fs.NewShareUri(res.ID, s.Password))
		if err != nil {
			return nil, serializer.NewError(serializer.CodeInternalSetting, "Invalid share url", err)
		}

		root, err := m.Get(c, shareUri)
		if err != nil {
			return nil, serializer.NewError(serializer.CodeNotFound, "File not found", err)
		}

		res.SourceUri = root.Uri(true).String()
	}

	return res, nil

}

func effectiveSharePermission(ctx context.Context, dep dependency.Dep, viewer *ent.User, share *ent.Share) inventory.SharePermission {
	if share == nil || share.Edges.User == nil {
		return inventory.SharePermission{}
	}
	if viewer != nil && viewer.ID == share.Edges.User.ID {
		return inventory.SharePermission{
			CanList: true, CanRead: true, CanUpdate: true,
			CanCreate: true, CanDelete: true, CanShare: true,
			Discoverable: true,
		}
	}

	acl, err := dep.ShareACLClient().List(ctx, share.ID)
	if err != nil {
		dep.Logger().Warning("Failed to load share ACL for share %d: %s", share.ID, err)
		return inventory.SharePermission{}
	}
	if len(acl.Builtin)+len(acl.Group)+len(acl.User) == 0 {
		return inventory.SharePermission{CanList: true, CanRead: true}
	}

	userID, userGroupID := 0, 0
	if viewer != nil {
		userID = viewer.ID
		if viewer.Edges.Group != nil {
			userGroupID = viewer.Edges.Group.ID
		}
	}
	ownerGroupID := 0
	if share.Edges.User.Edges.Group != nil {
		ownerGroupID = share.Edges.User.Edges.Group.ID
	}

	permission, matched := acl.EffectivePermission(userID, userGroupID, ownerGroupID, inventory.IsAnonymousUser(viewer))
	if !matched {
		return inventory.SharePermission{}
	}
	return permission
}

type (
	ListShareService struct {
		PageSize       int    `form:"page_size" binding:"required,min=10,max=100"`
		OrderBy        string `uri:"order_by" form:"order_by" json:"order_by"`
		OrderDirection string `uri:"order_direction" form:"order_direction" json:"order_direction"`
		NextPageToken  string `form:"next_page_token"`
	}
	ListShareParamCtx struct{}
)

func (s *ListShareService) List(c *gin.Context) (*ListShareResponse, error) {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)
	hasher := dep.HashIDEncoder()
	shareClient := dep.ShareClient()

	args := &inventory.ListShareArgs{
		PaginationArgs: &inventory.PaginationArgs{
			UseCursorPagination: true,
			PageToken:           s.NextPageToken,
			PageSize:            s.PageSize,
			Order:               inventory.OrderDirection(s.OrderDirection),
			OrderBy:             s.OrderBy,
		},
		UserID: user.ID,
	}

	ctx := context.WithValue(c, inventory.LoadShareUser{}, true)
	ctx = context.WithValue(ctx, inventory.LoadShareFile{}, true)
	ctx = context.WithValue(ctx, inventory.LoadFileMetadata{}, true)
	res, err := shareClient.List(ctx, args)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to list shares", err)
	}

	base := dep.SettingProvider().SiteURL(ctx)
	return BuildListShareResponse(ctx, res, hasher, base, user, true), nil
}

func (s *ListShareService) ListInUserProfile(c *gin.Context, uid int) (*ListShareResponse, error) {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)
	hasher := dep.HashIDEncoder()
	shareClient := dep.ShareClient()

	targetUser, err := dep.UserClient().GetActiveByID(c, uid)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to get user", err)
	}

	if targetUser.Settings != nil && targetUser.Settings.ShareLinksInProfile == types.ProfileHideShare {
		return nil, serializer.NewError(serializer.CodeParamErr, "User has disabled share links in profile", nil)
	}

	publicOnly := targetUser.Settings == nil || targetUser.Settings.ShareLinksInProfile == types.ProfilePublicShareOnly
	args := &inventory.ListShareArgs{
		PaginationArgs: &inventory.PaginationArgs{
			UseCursorPagination: true,
			PageToken:           s.NextPageToken,
			PageSize:            s.PageSize,
			Order:               inventory.OrderDirection(s.OrderDirection),
			OrderBy:             s.OrderBy,
		},
		UserID:     uid,
		PublicOnly: publicOnly,
	}

	ctx := context.WithValue(c, inventory.LoadShareUser{}, true)
	ctx = context.WithValue(ctx, inventory.LoadShareFile{}, true)
	ctx = context.WithValue(ctx, inventory.LoadFileMetadata{}, true)
	res, err := shareClient.List(ctx, args)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to list shares", err)
	}

	base := dep.SettingProvider().SiteURL(ctx)
	return BuildListShareResponse(ctx, res, hasher, base, user, false), nil
}
