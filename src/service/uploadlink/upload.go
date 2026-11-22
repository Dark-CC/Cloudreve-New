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
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/fs"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/manager"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/cloudreve/Cloudreve/v4/service/callback"
	"github.com/cloudreve/Cloudreve/v4/service/explorer"
	"github.com/gin-gonic/gin"
)

// CreateSessionService starts an upload for a visitor. The session is created as
// the folder owner so quota and ownership stay correct, while the returned
// callback secret is scoped to this one session.
type (
	CreateSessionService struct {
		Token        string `json:"token" binding:"required"`
		Name         string `json:"name" binding:"required"`
		Size         int64  `json:"size" binding:"min=0"`
		Folder       string `json:"folder"`
		LastModified int64  `json:"last_modified"`
		MimeType     string `json:"mime_type"`
	}
	CreateSessionParamCtx struct{}
)

func (s *CreateSessionService) Create(c *gin.Context) (*explorer.UploadSessionResponse, error) {
	dep := dependency.FromContext(c)
	claim, err := openLink(dep, s.Token)
	if err != nil {
		return nil, err
	}

	owner, err := dep.UserClient().GetLoginUserByID(c, claim.Owner)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeNotFound, "upload link owner no longer exists", err)
	}

	name, err := sanitizeName(s.Name)
	if err != nil {
		return nil, err
	}
	folder, err := joinFolder(claim.Path, s.Folder)
	if err != nil {
		return nil, err
	}
	if claim.MaxSize > 0 && s.Size > claim.MaxSize {
		return nil, serializer.NewError(serializer.CodeParamErr, "file exceeds the limit of this upload link", nil)
	}

	uri, err := fs.NewUriFromString("cloudreve://my/" + path.Join(folder, name))
	if err != nil {
		return nil, serializer.NewError(serializer.CodeParamErr, "invalid upload path", err)
	}

	req := &fs.UploadRequest{
		Props: &fs.UploadProps{
			Uri:                    uri,
			Size:                   s.Size,
			MimeType:               s.MimeType,
			PreferredStoragePolicy: claim.Policy,
		},
	}
	if s.LastModified > 0 {
		modified := time.UnixMilli(s.LastModified)
		req.Props.LastModified = &modified
	}

	ctx := ownerContext(c, owner)
	m := manager.NewFileManager(dep, owner)
	defer m.Recycle()

	credential, err := m.CreateUploadSession(ctx, req)
	if err != nil {
		return nil, err
	}

	return explorer.BuildUploadSessionResponse(credential, dep.HashIDEncoder()), nil
}

// CompleteService finishes a visitor upload. The callback secret is delivered
// only to the visitor that created the session, so it doubles as the proof that
// this request may finalize that session.
type (
	CompleteService struct {
		Token   string `uri:"token" binding:"required"`
		Session string `uri:"session" binding:"required"`
		Secret  string `uri:"secret" binding:"required"`
	}
	CompleteParamCtx struct{}
)

func (s *CompleteService) Complete(c *gin.Context) error {
	session, claim, err := s.loadSession(c)
	if err != nil {
		return err
	}

	dep := dependency.FromContext(c)
	owner, err := dep.UserClient().GetLoginUserByID(c, claim.Owner)
	if err != nil {
		return serializer.NewError(serializer.CodeNotFound, "upload link owner no longer exists", err)
	}

	ctx := ownerContext(c, owner)
	m := manager.NewFileManager(dep, owner)
	defer m.Recycle()
	if _, err := m.CompleteUpload(ctx, session); err != nil {
		return fmt.Errorf("failed to complete upload: %w", err)
	}

	return nil
}

// loadSession resolves the link and the referenced upload session, proving the
// caller may act on it. A public visitor only ever learns the callback secret
// of the session they created, so possession of it plus the link token is the
// authorization for both completion and ticket refresh.
func (s *CompleteService) loadSession(c *gin.Context) (*fs.UploadSession, *claims, error) {
	dep := dependency.FromContext(c)
	// The session already exists, so the link's expiry must not strand an
	// upload that started while it was still valid.
	claim, err := openLinkAt(dep, s.Token, true)
	if err != nil {
		return nil, nil, err
	}

	raw, ok := dep.KV().Get(manager.UploadSessionCachePrefix + s.Session)
	if !ok {
		return nil, nil, serializer.NewError(serializer.CodeUploadSessionExpired, "upload session does not exist or expired", nil)
	}
	session, ok := raw.(fs.UploadSession)
	if !ok {
		return nil, nil, serializer.NewError(serializer.CodeUploadSessionExpired, "upload session is invalid", nil)
	}
	if session.UID != claim.Owner {
		return nil, nil, serializer.NewError(serializer.CodeNoPermissionErr, "upload session does not belong to this link", nil)
	}
	if session.CallbackSecret != s.Secret {
		return nil, nil, serializer.NewError(serializer.CodeCredentialInvalid, "invalid upload callback secret", nil)
	}
	if !strings.HasPrefix(strings.Trim(session.Props.SavePath, "/"), strings.Trim(claim.Path, "/")) {
		return nil, nil, serializer.NewError(serializer.CodeNoPermissionErr, "upload session escaped the linked folder", nil)
	}

	return &session, claim, nil
}

// RefreshService re-issues the sealed 1yunpan ticket for a visitor upload. The
// visitor calls it when the edge reports an expired upstream session; without
// this a public upload would fail as soon as the embedded login ages out.
type (
	RefreshService struct {
		Token   string `uri:"token" binding:"required"`
		Session string `uri:"session" binding:"required"`
		Secret  string `uri:"secret" binding:"required"`
	}
	RefreshParamCtx struct{}
	RefreshResponse struct {
		EdgeTicket string `json:"edge_ticket"`
		Expires    int64  `json:"expires"`
	}
)

func (s *RefreshService) Refresh(c *gin.Context) (*RefreshResponse, error) {
	complete := &CompleteService{Token: s.Token, Session: s.Session, Secret: s.Secret}
	session, _, err := complete.loadSession(c)
	if err != nil {
		return nil, err
	}

	ticket, err := callback.RefreshEdgeTicket(c, session)
	if err != nil {
		return nil, err
	}

	return &RefreshResponse{
		EdgeTicket: ticket,
		Expires:    session.Props.ExpireAt.Unix(),
	}, nil
}

// ownerContext attaches the resolved owner to the request context so downstream
// services that read inventory.UserFromContext see the right account.
func ownerContext(c *gin.Context, owner *ent.User) context.Context {
	ctx := context.WithValue(c.Request.Context(), inventory.UserCtx{}, owner)
	ctx = context.WithValue(ctx, inventory.UserIDCtx{}, owner.ID)
	c.Request = c.Request.WithContext(ctx)
	return ctx
}
