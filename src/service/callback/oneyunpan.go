package callback

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/driver/oneyunpan"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/fs"
	"github.com/cloudreve/Cloudreve/v4/pkg/filemanager/manager"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/gin-gonic/gin"
)

// RefreshEdgeTicket opens the edge ticket stored with the upload session,
// refreshes the 1yunpan login, and returns a freshly sealed ticket. The caller
// has already proven it holds the callback secret, so the old login is only
// reused as a fallback when 1yunpan still accepts it.
func RefreshEdgeTicket(c *gin.Context, session *fs.UploadSession) (string, error) {
	if session == nil || session.Policy == nil {
		return "", serializer.NewError(serializer.CodeParamErr, "upload session is missing a storage policy", nil)
	}
	if session.Policy.Type != types.PolicyTypeOneyunpan {
		return "", serializer.NewError(serializer.CodePolicyNotAllowed, "", nil)
	}

	dep := dependency.FromContext(c)
	edge := oneyunpan.ResolveEdgeConfig(c, dep.SettingProvider(), session.Policy)
	if !edge.Enabled() {
		return "", serializer.NewError(serializer.CodePolicyNotAllowed, "edge endpoint is not configured", nil)
	}

	key, err := oneyunpan.DecodeTicketKey(edge.TicketKey)
	if err != nil {
		return "", serializer.NewError(serializer.CodeParamErr, err.Error(), err)
	}

	ctx := context.Background()

	driver, err := oneyunpan.New(
		ctx,
		session.Policy,
		dep.SettingProvider(),
		dep.ConfigProvider(),
		dep.Logger(),
		dep.MimeDetector(ctx),
	)
	if err != nil {
		return "", serializer.NewError(serializer.CodeDBError, "failed to create 1yunpan driver", err)
	}

	// Force a fresh login rather than trusting the cached session: this path is
	// only reached because the upstream rejected the previous one.
	driver.ForgetSession()
	sessionID, _, err := driver.Session(ctx)
	if err != nil {
		return "", serializer.NewError(serializer.CodeCredentialInvalid, "failed to refresh 1yunpan session", err)
	}
	if sessionID == "" {
		return "", serializer.NewError(serializer.CodeCredentialInvalid, "1yunpan returned an empty session", errors.New("empty session"))
	}

	refreshKey, err := oneyunpan.NewRefreshKey()
	if err != nil {
		return "", err
	}

	// Extend the Cloudreve upload session as well: a long edge upload may
	// outlive the original TTL, and the completion callback would otherwise be
	// rejected even though the upstream object is already in place.
	ttl := dep.SettingProvider().UploadSessionTTL(ctx)
	session.Props.ExpireAt = time.Now().Add(ttl)
	if err := dep.KV().Set(manager.UploadSessionCachePrefix+session.Props.UploadSessionID, *session, max(1, int(ttl.Seconds()))); err != nil {
		return "", serializer.NewError(serializer.CodeInternalSetting, "failed to extend upload session", err)
	}

	ticket, err := oneyunpan.SealTicketWithSite(key, edge.Site, &oneyunpan.EdgeTicket{
		Session:        sessionID,
		Path:           strings.Trim(session.Props.SavePath, "/"),
		Size:           session.Props.Size,
		UploadEndpoint: oneyunpan.UploadEndpointForPolicy(session.Policy),
		Expires:        session.Props.ExpireAt.Unix(),
		RefreshKey:     refreshKey,
	})
	if err != nil {
		return "", serializer.NewError(serializer.CodeInternalSetting, "failed to seal edge ticket", err)
	}

	return ticket, nil
}
