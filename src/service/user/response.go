package user

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/user"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/auth"
	"github.com/cloudreve/Cloudreve/v4/pkg/boolset"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/fs"
	"github.com/cloudreve/Cloudreve/v4/pkg/hashid"
	"github.com/cloudreve/Cloudreve/v4/service/audit"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/samber/lo"
	"github.com/ua-parser/uap-go/uaparser"
)

type PreparePasskeyLoginResponse struct {
	Options   *protocol.CredentialAssertion `json:"options"`
	SessionID string                        `json:"session_id"`
}

type UserSettings struct {
	VersionRetentionEnabled bool            `json:"version_retention_enabled"`
	VersionRetentionExt     []string        `json:"version_retention_ext,omitempty"`
	VersionRetentionMax     int             `json:"version_retention_max,omitempty"`
	Paswordless             bool            `json:"passwordless"`
	TwoFAEnabled            bool            `json:"two_fa_enabled"`
	Passkeys                []Passkey       `json:"passkeys,omitempty"`
	DisableViewSync         bool            `json:"disable_view_sync"`
	ShareLinksInProfile     string          `json:"share_links_in_profile"`
	GroupExpires            *time.Time      `json:"group_expires,omitempty"`
	OAuthGrants             []OauthGrant    `json:"oauth_grants,omitempty"`
	OpenID                  []OpenIDLink    `json:"open_id,omitempty"`
	LoginActivity           []LoginActivity `json:"login_activity,omitempty"`
	Credit                  int64           `json:"credit"`
	StoragePacks            []StoragePack   `json:"storage_packs,omitempty"`
}

// StoragePack is the public view of a purchased storage expansion.
type StoragePack struct {
	ID          int        `json:"id"`
	Name        string     `json:"name"`
	Size        int64      `json:"size"`
	ActiveSince *time.Time `json:"active_since,omitempty"`
	ExpireAt    *time.Time `json:"expire_at,omitempty"`
}

// LoginActivity is one entry in the recent sign-in table shown by Pro's
// security settings page.
type LoginActivity struct {
	Webdav         bool      `json:"webdav,omitempty"`
	LoginWith      string    `json:"login_with,omitempty"`
	Passkey        string    `json:"passkey,omitempty"`
	OpenIDProvider int       `json:"open_id_provider,omitempty"`
	Success        bool      `json:"success"`
	Browser        string    `json:"browser,omitempty"`
	OS             string    `json:"os,omitempty"`
	Device         string    `json:"device,omitempty"`
	IP             string    `json:"ip,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// OpenIDLink is the Pro-compatible linked identity descriptor.
type OpenIDLink struct {
	ID       int       `json:"id,omitempty"`
	Provider int       `json:"provider"`
	Sub      string    `json:"sub,omitempty"`
	Email    string    `json:"email,omitempty"`
	Name     string    `json:"name,omitempty"`
	Avatar   string    `json:"avatar,omitempty"`
	LinkedAt time.Time `json:"linked_at"`
}

func BuildUserSettings(u *ent.User, passkeys []*ent.Passkey, parser *uaparser.Parser, grants []*ent.OAuthGrant,
	membershipExpiry *time.Time, openIDs ...[]OpenIDLink) *UserSettings {
	var linked []OpenIDLink
	if len(openIDs) > 0 {
		linked = openIDs[0]
	}
	res := &UserSettings{
		VersionRetentionEnabled: u.Settings.VersionRetention,
		VersionRetentionExt:     u.Settings.VersionRetentionExt,
		VersionRetentionMax:     u.Settings.VersionRetentionMax,
		TwoFAEnabled:            u.TwoFactorSecret != "",
		Paswordless:             u.Password == "",
		Passkeys: lo.Map(passkeys, func(item *ent.Passkey, index int) Passkey {
			return BuildPasskey(item)
		}),
		DisableViewSync:     u.Settings.DisableViewSync,
		ShareLinksInProfile: string(u.Settings.ShareLinksInProfile),
		GroupExpires:        membershipExpiry,
		OAuthGrants: lo.Map(grants, func(item *ent.OAuthGrant, index int) OauthGrant {
			return BuildOauthGrant(item)
		}),
		OpenID: linked,
	}
	return res
}

// BuildLoginActivities converts login audit records into the compact shape
// consumed by the recent sign-in table. Failed attempts are retained so users
// can spot suspicious access.
func BuildLoginActivities(logs []*ent.AuditLog, parser *uaparser.Parser) []LoginActivity {
	res := make([]LoginActivity, 0, len(logs))
	for _, item := range logs {
		if item == nil {
			continue
		}
		activity := LoginActivity{
			IP:        item.IP,
			Success:   item.Type == int(audit.EventUserLogin),
			Webdav:    item.Type == int(audit.EventWebdavLoginFailed),
			CreatedAt: item.CreatedAt,
		}
		if item.Content != nil {
			activity.LoginWith = item.Content.LoginWith
			activity.Passkey = item.Content.Passkey
			activity.OpenIDProvider = item.Content.OpenIDProvider
			if activity.LoginWith == "" && activity.OpenIDProvider > 0 {
				activity.LoginWith = "openid"
			}
			if item.Content.UserAgent != "" && parser != nil {
				client := parser.Parse(item.Content.UserAgent)
				activity.Browser = client.UserAgent.Family
				activity.OS = client.Os.Family
				activity.Device = client.Device.Family
			}
		}
		if activity.LoginWith == "" && !activity.Webdav {
			activity.LoginWith = "password"
		}
		res = append(res, activity)
	}
	return res
}

type Passkey struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

func BuildPasskey(passkey *ent.Passkey) Passkey {
	return Passkey{
		ID:        passkey.CredentialID,
		Name:      passkey.Name,
		UsedAt:    passkey.UsedAt,
		CreatedAt: passkey.CreatedAt,
	}
}

// Node option for handling workflows.
type Node struct {
	ID           string              `json:"id"`
	Name         string              `json:"name"`
	Type         string              `json:"type"`
	Capabilities *boolset.BooleanSet `json:"capabilities"`
}

// StoragePolicy is the public storage policy option exposed to end users.
type StoragePolicy struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	MaxSize int64  `json:"max_size"`
}

// BuildNodes serialize a list of nodes.
func BuildNodes(nodes []*ent.Node, idEncoder hashid.Encoder) []*Node {
	res := make([]*Node, 0, len(nodes))
	for _, v := range nodes {
		res = append(res, BuildNode(v, idEncoder))
	}

	return res
}

// BuildNode serialize a node.
func BuildNode(node *ent.Node, idEncoder hashid.Encoder) *Node {
	return &Node{
		ID:           hashid.EncodeNodeID(idEncoder, node.ID),
		Name:         node.Name,
		Type:         string(node.Type),
		Capabilities: node.Capabilities,
	}
}

// BuiltinLoginResponse response for a successful login for builtin auth provider.
type BuiltinLoginResponse struct {
	User  User       `json:"user"`
	Token auth.Token `json:"token"`
}

// User 用户序列化器
type User struct {
	ID                  string                         `json:"id"`
	Email               string                         `json:"email,omitempty"`
	Nickname            string                         `json:"nickname"`
	Status              user.Status                    `json:"status,omitempty"`
	Avatar              string                         `json:"avatar,omitempty"`
	CreatedAt           time.Time                      `json:"created_at"`
	PreferredTheme      string                         `json:"preferred_theme,omitempty"`
	Anonymous           bool                           `json:"anonymous,omitempty"`
	Group               *Group                         `json:"group,omitempty"`
	Pined               []types.PinedFile              `json:"pined,omitempty"`
	Language            string                         `json:"language,omitempty"`
	DisableViewSync     bool                           `json:"disable_view_sync,omitempty"`
	ShareLinksInProfile types.ShareLinksInProfileLevel `json:"share_links_in_profile,omitempty"`
}

type Group struct {
	ID                  string              `json:"id"`
	Name                string              `json:"name"`
	Permission          *boolset.BooleanSet `json:"permission,omitempty"`
	DirectLinkBatchSize int                 `json:"direct_link_batch_size,omitempty"`
	TrashRetention      int                 `json:"trash_retention,omitempty"`
}

// GroupOption is the compact group shape used by Pro's group picker.
type GroupOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type storage struct {
	Used  uint64 `json:"used"`
	Free  uint64 `json:"free"`
	Total uint64 `json:"total"`
}

// WebAuthnCredentials 外部验证器凭证
type WebAuthnCredentials struct {
	ID          []byte `json:"id"`
	FingerPrint string `json:"fingerprint"`
}

type PrepareLoginResponse struct {
	WebAuthnEnabled bool  `json:"webauthn_enabled"`
	PasswordEnabled bool  `json:"password_enabled"`
	SSOEnabled      bool  `json:"sso_enabled,omitempty"`
	QQEnabled       bool  `json:"qq_enabled,omitempty"`
	OpenIDProviders []int `json:"openid_providers,omitempty"`
}

// BuildWebAuthnList 构建设置页面凭证列表
func BuildWebAuthnList(credentials []webauthn.Credential) []WebAuthnCredentials {
	res := make([]WebAuthnCredentials, 0, len(credentials))
	for _, v := range credentials {
		credential := WebAuthnCredentials{
			ID:          v.ID,
			FingerPrint: fmt.Sprintf("% X", v.Authenticator.AAGUID),
		}
		res = append(res, credential)
	}

	return res
}

// BuildUser 序列化用户
func BuildUser(user *ent.User, idEncoder hashid.Encoder) User {
	pined := user.Settings.Pined
	if group := user.Edges.Group; group != nil && group.Settings != nil && len(group.Settings.DefaultPinnedShares) > 0 {
		seen := make(map[string]struct{}, len(pined))
		for _, item := range pined {
			seen[item.Uri] = struct{}{}
		}
		defaults := make([]types.PinedFile, 0, len(group.Settings.DefaultPinnedShares))
		for _, item := range group.Settings.DefaultPinnedShares {
			if item.ShareID <= 0 {
				continue
			}
			uri := fs.NewShareUri(hashid.EncodeShareID(idEncoder, item.ShareID), "")
			if _, ok := seen[uri]; ok {
				continue
			}
			defaults = append(defaults, types.PinedFile{
				Uri:      uri,
				Name:     item.Name,
				Readonly: true,
			})
			seen[uri] = struct{}{}
		}
		pined = append(defaults, pined...)
	}

	return User{
		ID:                  hashid.EncodeUserID(idEncoder, user.ID),
		Email:               user.Email,
		Nickname:            user.Nick,
		Status:              user.Status,
		Avatar:              user.Avatar,
		CreatedAt:           user.CreatedAt,
		PreferredTheme:      user.Settings.PreferredTheme,
		Anonymous:           user.ID == 0,
		Group:               BuildGroup(user.Edges.Group, idEncoder),
		Pined:               pined,
		Language:            user.Settings.Language,
		DisableViewSync:     user.Settings.DisableViewSync,
		ShareLinksInProfile: user.Settings.ShareLinksInProfile,
	}
}

type OauthGrant struct {
	ClientID   string     `json:"client_id"`
	ClientName string     `json:"client_name"`
	ClientLogo string     `json:"client_logo"`
	Scopes     []string   `json:"scopes"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

func BuildOauthGrant(grant *ent.OAuthGrant) OauthGrant {
	res := OauthGrant{
		Scopes:     grant.Scopes,
		LastUsedAt: grant.LastUsedAt,
	}

	if grant.Edges.Client != nil {
		res.ClientID = grant.Edges.Client.GUID
		res.ClientName = grant.Edges.Client.Name
		if grant.Edges.Client.Props != nil {
			res.ClientLogo = grant.Edges.Client.Props.Icon
		}
	}

	return res
}

func BuildGroup(group *ent.Group, idEncoder hashid.Encoder) *Group {
	if group == nil {
		return nil
	}
	return &Group{
		ID:                  hashid.EncodeGroupID(idEncoder, group.ID),
		Name:                group.Name,
		Permission:          group.Permissions,
		DirectLinkBatchSize: group.Settings.SourceBatchSize,
		TrashRetention:      group.Settings.TrashRetention,
	}
}

// BuildGroupOption serializes a group for the Pro-compatible group list.
func BuildGroupOption(group *ent.Group, idEncoder hashid.Encoder) GroupOption {
	return GroupOption{
		ID:   hashid.EncodeGroupID(idEncoder, group.ID),
		Name: group.Name,
	}
}

const sensitiveTag = "redacted"

const (
	RedactLevelAnonymous = iota
	RedactLevelUser
)

// BuildUserRedacted Serialize a user without sensitive information.
// The email is only exposed when the requester is a signed-in user (level >=
// RedactLevelUser) and the `expose_user_email` setting is enabled.
func BuildUserRedacted(ctx context.Context, u *ent.User, level int, idEncoder hashid.Encoder) User {
	userRaw := BuildUser(u, idEncoder)

	user := User{
		ID:                  userRaw.ID,
		Nickname:            userRaw.Nickname,
		Avatar:              userRaw.Avatar,
		CreatedAt:           userRaw.CreatedAt,
		ShareLinksInProfile: userRaw.ShareLinksInProfile,
	}

	if userRaw.Group != nil {
		user.Group = RedactedGroup(userRaw.Group)
	}

	if level == RedactLevelUser && dependency.FromContext(ctx).SettingProvider().ExposeUserEmail(ctx) {
		user.Email = userRaw.Email
	}

	return user
}

// BuildGroupRedacted Serialize a group without sensitive information.
func RedactedGroup(g *Group) *Group {
	if g == nil {
		return nil
	}

	return &Group{
		ID:   g.ID,
		Name: g.Name,
	}
}
