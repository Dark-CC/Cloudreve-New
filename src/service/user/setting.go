package user

import (
	"context"
	"crypto/md5"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/auth"
	"github.com/cloudreve/Cloudreve/v4/pkg/hashid"
	"github.com/cloudreve/Cloudreve/v4/pkg/request"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/cloudreve/Cloudreve/v4/pkg/setting"
	"github.com/cloudreve/Cloudreve/v4/pkg/thumb"
	"github.com/cloudreve/Cloudreve/v4/pkg/util"
	"github.com/cloudreve/Cloudreve/v4/service/audit"
	"github.com/gin-gonic/gin"
	"github.com/pquerna/otp/totp"
)

const (
	twoFaEnableSessionKey = "2fa_init_"
)

// Init2FA 初始化二步验证
func Init2FA(c *gin.Context) (string, error) {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)

	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "Cloudreve",
		AccountName: user.Email,
	})
	if err != nil {
		return "", serializer.NewError(serializer.CodeInternalSetting, "Failed to generate TOTP secret", err)
	}

	if err := dep.KV().Set(fmt.Sprintf("%s%d", twoFaEnableSessionKey, user.ID), key.Secret(), 600); err != nil {
		return "", serializer.NewError(serializer.CodeInternalSetting, "Failed to store TOTP session", err)
	}

	return key.Secret(), nil
}

type (
	// AvatarService Service to get avatar
	GetAvatarService struct {
		NoCache bool `form:"nocache"`
	}
	GetAvatarServiceParamsCtx struct{}
)

const (
	GravatarAvatar = "gravatar"
	FileAvatar     = "file"
)

// Get 获取用户头像
func (service *GetAvatarService) Get(c *gin.Context) error {
	dep := dependency.FromContext(c)
	settings := dep.SettingProvider()
	// 查找目标用户
	uid := hashid.FromContext(c)
	userClient := dep.UserClient()
	user, err := userClient.GetByID(c, uid)

	if err != nil {
		return serializer.NewError(serializer.CodeUserNotFound, "", err)
	}

	if !service.NoCache {
		c.Header("Cache-Control", fmt.Sprintf("public, max-age=%d", settings.PublicResourceMaxAge(c)))
	}

	// 未设定头像时，返回404错误
	if user.Avatar == "" {
		c.Status(404)
		return nil
	}

	avatarSettings := settings.Avatar(c)

	// Gravatar 头像重定向
	if user.Avatar == GravatarAvatar {
		gravatarRoot, err := url.Parse(avatarSettings.Gravatar)
		if err != nil {
			return serializer.NewError(serializer.CodeInternalSetting, "Failed to parse Gravatar server", err)
		}
		email_lowered := strings.ToLower(user.Email)
		has := md5.Sum([]byte(email_lowered))
		avatar, _ := url.Parse(fmt.Sprintf("/avatar/%x?d=mm&s=200", has))

		c.Redirect(http.StatusFound, gravatarRoot.ResolveReference(avatar).String())
		return nil
	}

	// 本地文件头像
	if user.Avatar == FileAvatar {
		avatarRoot := util.DataPath(avatarSettings.Path)

		avatarPath := filepath.Join(avatarRoot, fmt.Sprintf("avatar_%d.png", user.ID))
		avatar, err := os.Open(avatarPath)
		if err != nil {
			dep.Logger().Warning("Failed to open avatar file", err)
			// Self-heal stale avatar references left behind by migration or
			// manual data changes so clients fall back to the default avatar.
			if _, updateErr := dep.UserClient().UpdateAvatar(c, user, ""); updateErr != nil {
				dep.Logger().Warning("Failed to reset missing avatar for user %d: %s", user.ID, updateErr)
			}
			c.Status(404)
			return nil
		}
		defer avatar.Close()

		http.ServeContent(c.Writer, c.Request, "avatar.png", user.UpdatedAt, avatar)
		return nil
	}

	c.Status(404)
	return nil
}

// cancelUserMembership immediately ends a paid group membership and restores
// the user's previous group. Pro exposes this as PATCH /user/setting with
// group_expires=true.
func cancelUserMembership(c *gin.Context, dep dependency.Dep, u *ent.User) error {
	client := dep.DBClient()
	if client == nil || client.Driver() == nil {
		return serializer.NewError(serializer.CodeDBError, "Database driver is unavailable", nil)
	}
	drv := client.Driver()

	query, args := rebindMembershipSQL(drv.Dialect(),
		"SELECT group_users, previous_group, group_expires FROM users WHERE id = ? AND deleted_at IS NULL LIMIT 1",
		u.ID)
	rows := new(entsql.Rows)
	if err := drv.Query(c, query, args, rows); err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to load membership", err)
	}
	if !rows.Next() {
		rows.Close()
		return serializer.NewError(serializer.CodeNotFound, "User not found", nil)
	}

	var (
		currentGroup  int
		previousGroup sql.NullInt64
		expiresRaw    any
	)
	if err := rows.Scan(&currentGroup, &previousGroup, &expiresRaw); err != nil {
		rows.Close()
		return serializer.NewError(serializer.CodeDBError, "Failed to read membership", err)
	}
	rows.Close()
	if expiresRaw == nil {
		return serializer.NewError(serializer.CodeParamErr, "No active membership to cancel", nil)
	}

	targetGroup := dep.SettingProvider().DefaultGroup(c)
	if previousGroup.Valid && previousGroup.Int64 > 0 {
		targetGroup = int(previousGroup.Int64)
	}
	if targetGroup <= 0 {
		return serializer.NewError(serializer.CodeInternalSetting, "No fallback group available", nil)
	}
	if _, err := dep.GroupClient().GetByID(c, targetGroup); err != nil {
		targetGroup = dep.SettingProvider().DefaultGroup(c)
		if targetGroup <= 0 {
			return serializer.NewError(serializer.CodeInternalSetting, "No fallback group available", nil)
		}
	}

	query, args = rebindMembershipSQL(drv.Dialect(),
		`UPDATE users SET group_users = ?, previous_group = NULL, group_expires = NULL
		 WHERE id = ? AND deleted_at IS NULL AND group_expires IS NOT NULL`,
		targetGroup, u.ID)
	var result sql.Result
	if err := drv.Exec(c, query, args, &result); err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to cancel membership", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to confirm membership cancellation", err)
	}
	if affected == 0 {
		return serializer.NewError(serializer.CodeParamErr, "No active membership to cancel", nil)
	}

	audit.Record(c, audit.EventMembershipUnsub, &audit.LogEntry{
		Reason:      "user canceled",
		GroupID:     targetGroup,
		GroupIDFrom: currentGroup,
	}, audit.WithUser(u.ID))
	return nil
}

func rebindMembershipSQL(dialect, query string, args ...any) (string, []any) {
	if dialect != "postgres" {
		return query, args
	}
	builder := strings.Builder{}
	index := 0
	for _, char := range query {
		if char == '?' {
			index++
			builder.WriteString("$" + strconv.Itoa(index))
			continue
		}
		builder.WriteRune(char)
	}
	return builder.String(), args
}

func getUserMembershipExpiry(c context.Context, dep dependency.Dep, userID int) (*time.Time, error) {
	client := dep.DBClient()
	if client == nil || client.Driver() == nil {
		return nil, nil
	}
	drv := client.Driver()
	query, args := rebindMembershipSQL(drv.Dialect(),
		"SELECT group_expires FROM users WHERE id = ? AND deleted_at IS NULL LIMIT 1", userID)
	rows := new(entsql.Rows)
	if err := drv.Query(c, query, args, rows); err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to load membership expiry", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil
	}
	var raw any
	if err := rows.Scan(&raw); err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to read membership expiry", err)
	}
	if raw == nil {
		return nil, nil
	}
	expiry, err := parseMembershipExpiry(raw)
	if err != nil {
		return nil, nil
	}
	return &expiry, nil
}

func parseMembershipExpiry(value any) (time.Time, error) {
	switch v := value.(type) {
	case time.Time:
		return v, nil
	case string:
		return parseMembershipExpiryString(v)
	case []byte:
		return parseMembershipExpiryString(string(v))
	case fmt.Stringer:
		return parseMembershipExpiryString(v.String())
	default:
		return time.Time{}, fmt.Errorf("unsupported expiry value %T", value)
	}
}

func parseMembershipExpiryString(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("empty expiry")
	}
	layouts := []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05-07",
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999-07",
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported expiry format %q", value)
}

// Settings 获取用户设定
func GetUserSettings(c *gin.Context) (*UserSettings, error) {
	dep := dependency.FromContext(c)
	u := inventory.UserFromContext(c)
	userClient := dep.UserClient()
	passkeys, err := userClient.ListPasskeys(c, u.ID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to get user passkey", err)
	}

	ctx := context.WithValue(c, inventory.LoadOAuthGrantClient{}, true)
	grants, err := dep.OAuthClientClient().GetGrantsByUserID(ctx, u.ID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to get user OAuth grants", err)
	}

	expiry, err := getUserMembershipExpiry(c, dep, u.ID)
	if err != nil {
		return nil, err
	}
	identities, err := dep.ExternalIdentityClient().ListByUser(c, u.ID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to get linked identities", err)
	}
	loginLogs, err := dep.AuditLogClient().List(c, &inventory.ListAuditLogArgs{
		PaginationArgs: &inventory.PaginationArgs{Page: 0, PageSize: 10, Order: inventory.OrderDirectionDesc},
		UserID:         u.ID,
		Types: []int{
			int(audit.EventUserLoginFailed),
			int(audit.EventUserLogin),
			int(audit.EventWebdavLoginFailed),
		},
	})
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to get recent sign-in activity", err)
	}
	openIDs := make([]OpenIDLink, 0, len(identities))
	for _, identity := range identities {
		providerID := openIDProviderID(c, dep, identity.Provider)
		openIDs = append(openIDs, OpenIDLink{
			ID:       identity.ID,
			Provider: providerID,
			Sub:      identity.Subject,
			Email:    identity.Email,
			Name:     identity.DisplayName,
			Avatar:   identity.Avatar,
			LinkedAt: identity.CreatedAt,
		})
	}
	settings := BuildUserSettings(u, passkeys, dep.UAParser(), grants, expiry, openIDs)
	settings.LoginActivity = BuildLoginActivities(loginLogs.Logs, dep.UAParser())
	if credit, creditErr := dep.VASClient().Credit(c, u.ID); creditErr == nil {
		settings.Credit = credit
	}
	if packs, packErr := dep.VASClient().ListStoragePacks(c, u.ID); packErr == nil {
		settings.StoragePacks = make([]StoragePack, 0, len(packs))
		for _, pack := range packs {
			expireAt := pack.ExpireAt
			if expireAt != nil && expireAt.IsZero() {
				expireAt = nil
			}
			settings.StoragePacks = append(settings.StoragePacks, StoragePack{
				ID:          pack.ID,
				Name:        pack.Name,
				Size:        pack.Size,
				ActiveSince: pack.ActiveSince,
				ExpireAt:    expireAt,
			})
		}
	}
	return settings, nil

	// 用户组有效期

	//return serializer.Response{
	//	Data: map[string]interface{}{
	//		"uid":           user.ID,
	//		"qq":            user.OpenID != "",
	//		"homepage":      !user.OptionsSerialized.ProfileOff,
	//		"two_factor":    user.TwoFactor != "",
	//		"prefer_theme":  user.OptionsSerialized.PreferredTheme,
	//		"themes":        model.GetSettingByName("themes"),
	//		"group_expires": groupExpires,
	//		"authn":         serializer.BuildWebAuthnList(user.WebAuthnCredentials()),
	//	},
	//}
}

func openIDProviderID(c *gin.Context, dep dependency.Dep, providerType string) int {
	providerType, storedID := setting.ParseExternalIdentityProviderKey(providerType)
	if storedID >= 0 {
		if provider := dep.SettingProvider().ExternalLoginProvider(c, storedID); provider != nil &&
			provider.Type == providerType {
			return storedID
		}
		return storedID
	}
	for _, provider := range dep.SettingProvider().ExternalLoginProviders(c) {
		if provider.Type == providerType {
			return provider.ID
		}
	}
	switch providerType {
	case "logto":
		return 0
	case "qq":
		return 1
	case "oidc":
		return 2
	default:
		return 0
	}
}

// GetRelocatePolicies returns storage policies available to the current
// user's group for relocation. Load-balance policies are not directly
// selectable targets, matching Pro's UI.
func GetRelocatePolicies(c *gin.Context) ([]*StoragePolicy, error) {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)
	if user.Edges.Group == nil {
		return nil, serializer.NewError(serializer.CodeGroupNotAllowed, "User group is not loaded", nil)
	}
	policyIDs, err := dep.GroupPolicyClient().ListGroupPolicies(c, user.Edges.Group.ID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to list group storage policies", err)
	}
	if len(policyIDs) == 0 {
		policy, err := dep.StoragePolicyClient().GetByGroup(c, user.Edges.Group)
		if err != nil {
			return nil, serializer.NewError(serializer.CodeDBError, "Failed to get group storage policy", err)
		}
		policyIDs = []int{policy.ID}
	}

	policies := make([]*StoragePolicy, 0, len(policyIDs))
	for _, policyID := range policyIDs {
		policy, err := dep.StoragePolicyClient().GetPolicyByID(c, policyID)
		if err != nil {
			continue
		}
		if policy.Type == string(types.PolicyTypeLoadBalance) {
			continue
		}
		policies = append(policies, &StoragePolicy{
			ID:      hashid.EncodePolicyID(dep.HashIDEncoder(), policy.ID),
			Name:    policy.Name,
			Type:    policy.Type,
			MaxSize: policy.MaxSize,
		})
	}

	return policies, nil
}

// GetAvailableNodes returns active nodes available to the current user's
// group for workflow execution.
func GetAvailableNodes(c *gin.Context) ([]*Node, error) {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)
	if user.Edges.Group == nil {
		return nil, serializer.NewError(serializer.CodeGroupNotAllowed, "User group is not loaded", nil)
	}
	if !user.Edges.Group.Permissions.Enabled(int(types.GroupPermissionSelectNode)) {
		return []*Node{}, nil
	}

	var subset []int
	if user.Edges.Group.Settings != nil {
		subset = user.Edges.Group.Settings.AvailableNodes
	}
	nodes, err := dep.NodeClient().ListActiveNodes(c, subset)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to list available nodes", err)
	}

	return BuildNodes(nodes, dep.HashIDEncoder()), nil
}

func UpdateUserAvatar(c *gin.Context) error {
	dep := dependency.FromContext(c)
	u := inventory.UserFromContext(c)
	settings := dep.SettingProvider()

	avatarSettings := settings.AvatarProcess(c)
	if c.Request.ContentLength == -1 || c.Request.ContentLength > avatarSettings.MaxFileSize {
		request.BlackHole(c.Request.Body)
		return serializer.NewError(serializer.CodeFileTooLarge, "", nil)
	}

	if c.Request.ContentLength == 0 {
		// Use Gravatar for empty body
		if _, err := dep.UserClient().UpdateAvatar(c, u, GravatarAvatar); err != nil {
			return serializer.NewError(serializer.CodeDBError, "Failed to update user avatar", err)
		}

		audit.Record(c, audit.EventChangeAvatar, &audit.LogEntry{Name: GravatarAvatar}, audit.WithUser(u.ID))
		return nil
	}

	err := updateAvatarFile(c, u, c.GetHeader("Content-Type"), c.Request.Body, avatarSettings)
	entry := &audit.LogEntry{Name: FileAvatar}
	if err != nil {
		entry.Failed = true
		entry.Error = err.Error()
	}
	audit.Record(c, audit.EventChangeAvatar, entry, audit.WithUser(u.ID))
	return err
}

func updateAvatarFile(ctx context.Context, u *ent.User, contentType string, file io.Reader, avatarSettings *setting.AvatarProcess) error {
	dep := dependency.FromContext(ctx)
	// Detect ext from content type
	ext := "png"
	switch contentType {
	case "image/jpeg", "image/jpg":
		ext = "jpg"
	case "image/gif":
		ext = "gif"
	}
	avatar, err := thumb.NewThumbFromFile(file, ext)
	if err != nil {
		return serializer.NewError(serializer.CodeParamErr, "Invalid image", err)
	}

	// Resize and save avatar
	avatar.CreateAvatar(avatarSettings.MaxWidth)
	avatarRoot := util.DataPath(avatarSettings.Path)
	f, err := util.CreatNestedFile(filepath.Join(avatarRoot, fmt.Sprintf("avatar_%d.png", u.ID)))
	if err != nil {
		return serializer.NewError(serializer.CodeIOFailed, "Failed to create avatar file", err)
	}

	defer f.Close()
	if err := avatar.Save(f, &setting.ThumbEncode{
		Quality: 100,
		Format:  "png",
	}); err != nil {
		return serializer.NewError(serializer.CodeIOFailed, "Failed to save avatar file", err)
	}

	if _, err := dep.UserClient().UpdateAvatar(ctx, u, FileAvatar); err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to update user avatar", err)
	}

	return nil
}

type (
	PatchUserSetting struct {
		Nick                    *string   `json:"nick" binding:"omitempty,min=1,max=255"`
		Language                *string   `json:"language" binding:"omitempty,min=1,max=255"`
		PreferredTheme          *string   `json:"preferred_theme" binding:"omitempty,hexcolor|rgb|rgba|hsl"`
		VersionRetentionEnabled *bool     `json:"version_retention_enabled" binding:"omitempty"`
		VersionRetentionExt     *[]string `json:"version_retention_ext" binding:"omitempty"`
		VersionRetentionMax     *int      `json:"version_retention_max" binding:"omitempty,min=0"`
		CurrentPassword         *string   `json:"current_password" binding:"omitempty,min=4,max=128"`
		NewPassword             *string   `json:"new_password" binding:"omitempty,min=6,max=128"`
		TwoFAEnabled            *bool     `json:"two_fa_enabled" binding:"omitempty"`
		TwoFACode               *string   `json:"two_fa_code" binding:"omitempty"`
		DisableViewSync         *bool     `json:"disable_view_sync" binding:"omitempty"`
		ShareLinksInProfile     *string   `json:"share_links_in_profile" binding:"omitempty"`
		GroupExpires            *bool     `json:"group_expires" binding:"omitempty"`
	}
	PatchUserSettingParamsCtx struct{}
)

func (s *PatchUserSetting) Patch(c *gin.Context) error {
	dep := dependency.FromContext(c)
	u := inventory.UserFromContext(c)
	userClient := dep.UserClient()
	saveSetting := false

	if s.Nick != nil {
		originalName := u.Nick
		if _, err := userClient.UpdateNickname(c, u, *s.Nick); err != nil {
			return serializer.NewError(serializer.CodeDBError, "Failed to update user nick", err)
		}
		audit.Record(c, audit.EventChangeNick, &audit.LogEntry{
			OriginalName: originalName,
			NewName:      *s.Nick,
		}, audit.WithUser(u.ID))
	}

	if s.Language != nil {
		u.Settings.Language = *s.Language
		saveSetting = true
	}

	if s.PreferredTheme != nil {
		u.Settings.PreferredTheme = *s.PreferredTheme
		saveSetting = true
	}

	if s.VersionRetentionEnabled != nil {
		u.Settings.VersionRetention = *s.VersionRetentionEnabled
		saveSetting = true
	}

	if s.VersionRetentionExt != nil {
		u.Settings.VersionRetentionExt = *s.VersionRetentionExt
		saveSetting = true
	}

	if s.VersionRetentionMax != nil {
		u.Settings.VersionRetentionMax = *s.VersionRetentionMax
		saveSetting = true
	}

	if s.DisableViewSync != nil {
		u.Settings.DisableViewSync = *s.DisableViewSync
		saveSetting = true
	}

	if s.ShareLinksInProfile != nil {
		u.Settings.ShareLinksInProfile = types.ShareLinksInProfileLevel(*s.ShareLinksInProfile)
		saveSetting = true
	}

	if s.GroupExpires != nil && *s.GroupExpires {
		if err := cancelUserMembership(c, dep, u); err != nil {
			return err
		}
	}

	if s.CurrentPassword != nil && s.NewPassword != nil {
		if err := auth.CheckScope(c, types.ScopeUserSecurityInfoWrite); err != nil {
			return err
		}

		if err := inventory.CheckPassword(u, *s.CurrentPassword); err != nil {
			audit.Record(c, audit.EventChangePassword, &audit.LogEntry{
				Failed: true,
				Error:  err.Error(),
			}, audit.WithUser(u.ID))
			return serializer.NewError(serializer.CodeIncorrectPassword, "Incorrect password", err)
		}

		if _, err := userClient.UpdatePassword(c, u, *s.NewPassword); err != nil {
			audit.Record(c, audit.EventChangePassword, &audit.LogEntry{
				Failed: true,
				Error:  err.Error(),
			}, audit.WithUser(u.ID))
			return serializer.NewError(serializer.CodeDBError, "Failed to update user password", err)
		}
		audit.Record(c, audit.EventChangePassword, &audit.LogEntry{}, audit.WithUser(u.ID))
	}

	if s.TwoFAEnabled != nil {
		if err := auth.CheckScope(c, types.ScopeUserSecurityInfoWrite); err != nil {
			return err
		}

		if *s.TwoFAEnabled {
			kv := dep.KV()
			secret, ok := kv.Get(fmt.Sprintf("%s%d", twoFaEnableSessionKey, u.ID))
			if !ok {
				return serializer.NewError(serializer.CodeInternalSetting, "You have not initiated 2FA session", nil)
			}

			if !totp.Validate(*s.TwoFACode, secret.(string)) {
				audit.Record(c, audit.EventEnable2FA, &audit.LogEntry{
					Failed: true,
					Error:  "Incorrect 2FA code",
				}, audit.WithUser(u.ID))
				return serializer.NewError(serializer.Code2FACodeErr, "Incorrect 2FA code", nil)
			}

			if _, err := userClient.UpdateTwoFASecret(c, u, secret.(string)); err != nil {
				audit.Record(c, audit.EventEnable2FA, &audit.LogEntry{
					Failed: true,
					Error:  err.Error(),
				}, audit.WithUser(u.ID))
				return serializer.NewError(serializer.CodeDBError, "Failed to update user 2FA", err)
			}
			audit.Record(c, audit.EventEnable2FA, &audit.LogEntry{}, audit.WithUser(u.ID))

		} else {
			if !totp.Validate(*s.TwoFACode, u.TwoFactorSecret) {
				audit.Record(c, audit.EventDisable2FA, &audit.LogEntry{
					Failed: true,
					Error:  "Incorrect 2FA code",
				}, audit.WithUser(u.ID))
				return serializer.NewError(serializer.Code2FACodeErr, "Incorrect 2FA code", nil)
			}

			if _, err := userClient.UpdateTwoFASecret(c, u, ""); err != nil {
				audit.Record(c, audit.EventDisable2FA, &audit.LogEntry{
					Failed: true,
					Error:  err.Error(),
				}, audit.WithUser(u.ID))
				return serializer.NewError(serializer.CodeDBError, "Failed to update user 2FA", err)
			}
			audit.Record(c, audit.EventDisable2FA, &audit.LogEntry{}, audit.WithUser(u.ID))

		}
	}

	if saveSetting {
		if err := userClient.SaveSettings(c, u); err != nil {
			return serializer.NewError(serializer.CodeDBError, "Failed to update user settings", err)
		}
	}

	return nil
}
