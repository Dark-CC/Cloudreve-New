package share

import (
	"context"
	"strconv"
	"strings"

	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent"
	userent "github.com/cloudreve/Cloudreve/v4/ent/user"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/pkg/hashid"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/cloudreve/Cloudreve/v4/service/audit"
	"github.com/gin-gonic/gin"
)

const (
	// MaxReportsPerUserPerDay limits how many reports a single logged-in user can submit in 24h.
	MaxReportsPerUserPerDay = 20
	// MaxDescriptionLength limits the report description length.
	MaxDescriptionLength = 2000
	// MaxFileURILength limits the reported file URI length.
	MaxFileURILength = 2048
)

type (
	// AbuseReportService submits a report against a share link or a user.
	AbuseReportService struct {
		Category    int    `json:"category"`
		Description string `json:"description"`
		FileURI     string `json:"file_uri"`
		Password    string `json:"password"`
		ShareID     string `json:"share_id"`
		UserID      string `json:"user_id"`
	}
	AbuseReportParamCtx struct{}
)

func (s *AbuseReportService) Report(c *gin.Context) error {
	return s.submit(c, hashid.FromContext(c))
}

// ReportSite submits a report via the site-wide endpoint. The target is
// identified by share_id or user_id in the request body.
func (s *AbuseReportService) ReportSite(c *gin.Context) error {
	dep := dependency.FromContext(c)
	hasher := dep.HashIDEncoder()

	shareID, err := decodeOptionalID(s.ShareID, hasher, hashid.ShareID)
	if err != nil {
		return serializer.NewError(serializer.CodeParamErr, "Invalid share ID", err)
	}

	userID, err := decodeOptionalID(s.UserID, hasher, hashid.UserID)
	if err != nil {
		return serializer.NewError(serializer.CodeParamErr, "Invalid user ID", err)
	}

	return s.submitTarget(c, shareID, userID)
}

func (s *AbuseReportService) submit(c *gin.Context, shareID int) error {
	return s.submitTarget(c, shareID, 0)
}

func decodeOptionalID(raw string, hasher hashid.Encoder, idType int) (int, error) {
	if raw == "" {
		return 0, nil
	}
	if id, err := strconv.Atoi(raw); err == nil {
		return id, nil
	}
	return hasher.Decode(raw, idType)
}

func (s *AbuseReportService) submitTarget(c *gin.Context, shareID, userID int) error {
	dep := dependency.FromContext(c)
	client := dep.AbuseReportClient()
	shareClient := dep.ShareClient()

	if s.Category < 0 || s.Category > 255 {
		return serializer.NewError(serializer.CodeParamErr, "Invalid report category", nil)
	}

	if len(s.Description) > MaxDescriptionLength {
		return serializer.NewError(serializer.CodeParamErr, "Description is too long", nil)
	}

	if len(s.FileURI) > MaxFileURILength {
		return serializer.NewError(serializer.CodeParamErr, "File URI is too long", nil)
	}

	if shareID <= 0 && userID <= 0 {
		return serializer.NewError(serializer.CodeParamErr, "Either share_id or user_id is required", nil)
	}

	if strings.TrimSpace(s.Description) == "" && s.FileURI == "" {
		return serializer.NewError(serializer.CodeParamErr, "Description is required", nil)
	}

	report := &ent.AbuseReport{
		Category:    s.Category,
		Description: strings.TrimSpace(s.Description),
		FileURI:     s.FileURI,
	}

	if shareID > 0 {
		ctx := context.WithValue(c, inventory.LoadShareUser{}, true)
		ctx = context.WithValue(ctx, inventory.LoadShareFile{}, true)
		share, err := shareClient.GetByID(ctx, shareID)
		if err != nil {
			if ent.IsNotFound(err) {
				return serializer.NewError(serializer.CodeNotFound, "Share not found", nil)
			}
			return serializer.NewError(serializer.CodeDBError, "Failed to get share", err)
		}

		if err := inventory.IsValidShare(share); err != nil {
			return serializer.NewError(serializer.CodeNotFound, "Share link expired", err)
		}

		// Reports for password-protected shares require the correct password.
		if share.Password != "" && s.Password != share.Password {
			reporter := inventory.UserFromContext(c)
			if share.Edges.User == nil || reporter == nil || share.Edges.User.ID != reporter.ID {
				return serializer.NewError(serializer.CodeShareLinkNotFound, "Share link password is incorrect", nil)
			}
		}

		report.ShareID = &share.ID
		if share.Edges.User != nil {
			report.ReportedUserID = &share.Edges.User.ID
		}
	}

	if userID > 0 {
		user, err := dep.UserClient().GetByID(c, userID)
		if err != nil {
			if ent.IsNotFound(err) {
				return serializer.NewError(serializer.CodeUserNotFound, "User not found", nil)
			}
			return serializer.NewError(serializer.CodeDBError, "Failed to get user", err)
		}
		if inventory.IsAnonymousUser(user) || user.Status != userent.StatusActive {
			return serializer.NewError(serializer.CodeUserNotFound, "User not found", nil)
		}
		report.ReportedUserID = &user.ID
	}

	reporter := inventory.UserFromContext(c)
	if reporter != nil && reporter.ID > 0 {
		if report.ReportedUserID != nil && *report.ReportedUserID == reporter.ID && report.ShareID == nil {
			return serializer.NewError(serializer.CodeParamErr, "Cannot report yourself", nil)
		}
		count, err := client.CountByReporter(c, reporter.ID)
		if err != nil {
			return serializer.NewError(serializer.CodeDBError, "Failed to check report quota", err)
		}
		if count >= MaxReportsPerUserPerDay {
			return serializer.NewError(serializer.CodeParamErr, "Too many reports, please try again later", nil)
		}
		report.ReporterID = &reporter.ID
	}

	created, err := client.Create(c, report)
	if err != nil {
		audit.Record(c, audit.EventReportAbuse, &audit.LogEntry{
			Failed: true,
			Error:  err.Error(),
			Reason: strconv.Itoa(s.Category),
			From:   s.FileURI,
		})
		return serializer.NewError(serializer.CodeDBError, "Failed to submit report", err)
	}

	opts := []audit.Option{}
	if created != nil && created.ShareID != nil {
		opts = append(opts, audit.WithShare(*created.ShareID))
	}
	audit.Record(c, audit.EventReportAbuse, &audit.LogEntry{
		Reason: strconv.Itoa(s.Category),
		From:   s.FileURI,
		Name:   s.Description,
	}, opts...)
	return nil
}
