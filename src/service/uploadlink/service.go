package uploadlink

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/driver/oneyunpan"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/fs"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/manager"
	"github.com/cloudreve/Cloudreve/v4/pkg/hashid"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/gin-gonic/gin"
)

// CreateService issues a public upload link for one folder.
type (
	CreateService struct {
		URI      string `json:"uri" binding:"required"`
		PolicyID string `json:"policy_id"`
		// Expires is a duration in seconds; 0 uses the default TTL.
		Expires int64 `json:"expires"`
		// MaxSize optionally caps a single upload, in bytes.
		MaxSize int64  `json:"max_size"`
		Note    string `json:"note"`
	}
	CreateParamCtx struct{}

	CreateResponse struct {
		Token     string `json:"token"`
		URL       string `json:"url"`
		Expires   int64  `json:"expires"`
		ExpiresAt int64  `json:"expires_at"`
		MaxSize   int64  `json:"max_size,omitempty"`
		Note      string `json:"note,omitempty"`
	}
)

func (s *CreateService) Create(c *gin.Context) (*CreateResponse, error) {
	dep := dependency.FromContext(c)
	user := inventory.UserFromContext(c)
	if user == nil || inventory.IsAnonymousUser(user) {
		return nil, serializer.NewError(serializer.CodeCheckLogin, "login required", nil)
	}

	uri, err := fs.NewUriFromString(s.URI)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeParamErr, "invalid folder uri", err)
	}
	if uri.FileSystem() != "my" {
		return nil, serializer.NewError(serializer.CodeParamErr, "upload links are only available in your own file system", nil)
	}

	policyID := 0
	if s.PolicyID != "" {
		policyID, err = dep.HashIDEncoder().Decode(s.PolicyID, hashid.PolicyID)
		if err != nil {
			return nil, serializer.NewError(serializer.CodeParamErr, "invalid policy id", err)
		}
	}

	m := manager.NewFileManager(dep, user)
	defer m.Recycle()
	target, err := m.Get(c, uri)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeNotFound, "folder does not exist", err)
	}
	if target.Type() != types.FileTypeFolder {
		return nil, serializer.NewError(serializer.CodeParamErr, "upload links can only be created for folders", nil)
	}
	if target.OwnerID() != user.ID {
		return nil, serializer.NewError(serializer.CodeNoPermissionErr, "you can only share folders you own", nil)
	}

	// Pin an edge-capable policy onto the link. Without this the upload path
	// picks a random policy from the group, so an anonymous link could land on
	// a local-disk policy that cannot produce an upload ticket at all.
	policy, err := resolveLinkPolicy(c, dep, user, target, policyID)
	if err != nil {
		return nil, err
	}
	policyID = policy.ID

	ttl := time.Duration(s.Expires) * time.Second
	if ttl <= 0 {
		ttl = defaultLinkTTL
	}
	if ttl > maxLinkTTL {
		ttl = maxLinkTTL
	}
	expiresAt := time.Now().Add(ttl)

	sealed, err := sealLink(dep, &claims{
		Owner:   user.ID,
		Path:    uri.PathTrimmed(),
		Policy:  policyID,
		Expires: expiresAt.Unix(),
		MaxSize: s.MaxSize,
		Note:    s.Note,
	})
	if err != nil {
		return nil, serializer.NewError(serializer.CodeInternalSetting, "failed to sign upload link", err)
	}

	base := strings.TrimSuffix(dep.SettingProvider().SiteURL(c).String(), "/")
	return &CreateResponse{
		Token:     sealed,
		URL:       fmt.Sprintf("%s/upload/%s", base, sealed),
		Expires:   int64(ttl.Seconds()),
		ExpiresAt: expiresAt.Unix(),
		MaxSize:   s.MaxSize,
		Note:      s.Note,
	}, nil
}

// resolveLinkPolicy picks the storage policy an anonymous upload will use.
// Public links upload through the edge, so only 1yunpan policies with an edge
// endpoint can serve them. An explicit choice is honored; otherwise the
// folder's own policy is used when possible, then any edge-capable policy
// bound to the owner's group.
func resolveLinkPolicy(
	ctx context.Context,
	dep dependency.Dep,
	user *ent.User,
	folder fs.File,
	explicit int,
) (*ent.StoragePolicy, error) {
	if explicit > 0 {
		policy, err := dep.StoragePolicyClient().GetPolicyByID(ctx, explicit)
		if err != nil {
			return nil, serializer.NewError(serializer.CodeDBError, "failed to load the storage policy", err)
		}
		if !SupportsLinkUpload(dep, ctx, policy) {
			return nil, serializer.NewError(
				serializer.CodePolicyNotAllowed,
				"public upload links require an edge-enabled 1yunpan storage policy",
				nil,
			)
		}
		return policy, nil
	}

	if folder != nil && folder.PolicyID() > 0 {
		if policy, err := dep.StoragePolicyClient().GetPolicyByID(ctx, folder.PolicyID()); err == nil && SupportsLinkUpload(dep, ctx, policy) {
			return policy, nil
		}
	}

	group := user.Edges.Group
	if group == nil {
		return nil, serializer.NewError(serializer.CodePolicyNotAllowed, "the owner has no user group", nil)
	}

	if ids, err := dep.GroupPolicyClient().ListGroupPolicies(ctx, group.ID); err == nil {
		for _, id := range ids {
			policy, err := dep.StoragePolicyClient().GetPolicyByID(ctx, id)
			if err != nil || !SupportsLinkUpload(dep, ctx, policy) {
				continue
			}
			return policy, nil
		}
	}
	if policy, err := dep.StoragePolicyClient().GetByGroup(ctx, group); err == nil && SupportsLinkUpload(dep, ctx, policy) {
		return policy, nil
	}

	return nil, serializer.NewError(
		serializer.CodePolicyNotAllowed,
		"no edge-enabled 1yunpan storage policy is available to this user",
		nil,
	)
}

// SupportsLinkUpload reports whether a policy can issue the sealed edge ticket
// public uploads depend on. The edge itself is configured site-wide, so the
// check consults the resolved configuration rather than the policy copy.
func SupportsLinkUpload(dep dependency.Dep, ctx context.Context, policy *ent.StoragePolicy) bool {
	return policy != nil &&
		policy.Type == string(types.PolicyTypeOneyunpan) &&
		oneyunpan.ResolveEdgeConfig(ctx, dep.SettingProvider(), policy).Enabled()
}

// InfoService returns the public metadata of a link so the upload page can show
// where files go without exposing the folder path itself.
type (
	InfoService struct {
		Token string `uri:"token" binding:"required"`
	}
	InfoParamCtx struct{}
	InfoResponse struct {
		Owner     string `json:"owner"`
		ExpiresAt int64  `json:"expires_at"`
		MaxSize   int64  `json:"max_size"`
		Note      string `json:"note,omitempty"`
	}
)

func (s *InfoService) Info(c *gin.Context) (*InfoResponse, error) {
	dep := dependency.FromContext(c)
	claim, err := openLink(dep, s.Token)
	if err != nil {
		return nil, err
	}

	owner, err := dep.UserClient().GetLoginUserByID(c, claim.Owner)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeNotFound, "upload link owner no longer exists", err)
	}

	maxSize := claim.MaxSize
	if maxSize <= 0 {
		maxSize = ownerGroupPolicyMaxSize(c, dep, owner.ID)
	}

	return &InfoResponse{
		Owner:     owner.Nick,
		ExpiresAt: claim.Expires,
		MaxSize:   maxSize,
		Note:      claim.Note,
	}, nil
}

// ownerGroupPolicyMaxSize returns the configured per-file limit for the owner's
// group so a public page can reject oversized files before uploading.
func ownerGroupPolicyMaxSize(ctx context.Context, dep dependency.Dep, uid int) int64 {
	owner, err := dep.UserClient().GetLoginUserByID(ctx, uid)
	if err != nil || owner.Edges.Group == nil {
		return 0
	}
	policy := owner.Edges.Group.Edges.StoragePolicies
	if policy == nil {
		return 0
	}
	return policy.MaxSize
}

// linkRoot resolves a token to the owner and normalized destination folder.
func linkRoot(ctx context.Context, dep dependency.Dep, token string) (*fs.URI, *claims, error) {
	claim, err := openLink(dep, token)
	if err != nil {
		return nil, nil, err
	}
	if _, err := dep.UserClient().GetLoginUserByID(ctx, claim.Owner); err != nil {
		return nil, nil, serializer.NewError(serializer.CodeNotFound, "upload link owner no longer exists", err)
	}

	uri, err := fs.NewUriFromString(fmt.Sprintf("cloudreve://my%s", claim.Path))
	if err != nil {
		return nil, nil, ErrLinkInvalid
	}
	return uri, claim, nil
}

// sanitizeName validates a browser-supplied file name and strips any directory
// component so a link can never escape its destination folder.
func sanitizeName(raw string) (string, error) {
	name := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "\x00") {
		return "", serializer.NewError(serializer.CodeParamErr, "invalid file name", nil)
	}
	if len(name) > 255 {
		return "", serializer.NewError(serializer.CodeParamErr, "file name is too long", nil)
	}
	return name, nil
}

// joinFolder appends an optional sub folder while keeping it inside the link.
func joinFolder(base, sub string) (string, error) {
	sub = strings.TrimSpace(strings.ReplaceAll(sub, "\\", "/"))
	sub = strings.Trim(sub, "/")
	if sub == "" {
		return strings.Trim(base, "/"), nil
	}
	clean := path.Clean(sub)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", serializer.NewError(serializer.CodeParamErr, "invalid folder", nil)
	}
	return strings.Trim(path.Join(base, clean), "/"), nil
}
