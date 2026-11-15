package admin

import (
	"context"
	"strconv"

	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/pkg/hashid"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

const (
	abuseCategoryCondition     = "abuse_category"
	abuseShareIDCondition      = "abuse_share_id"
	abuseReportedUserCondition = "abuse_reported_user_id"
	abuseReporterCondition     = "abuse_reporter_id"
	abuseFileURICondition      = "abuse_file_uri"

	// Pro compatible condition keys.
	abuseProCategoryCondition     = "abuse_report_category"
	abuseProShareIDCondition      = "abuse_report_share"
	abuseProReportedUserCondition = "abuse_report_user"
	abuseProReporterCondition     = "abuse_report_reporter"
	abuseProFileURICondition      = "abuse_report_file_uri"
)

func abuseCondition(conditions map[string]string, keys ...string) string {
	for _, key := range keys {
		if v := conditions[key]; v != "" {
			return v
		}
	}
	return ""
}

func (s *AdminListService) AbuseReports(c *gin.Context) (*ListAbuseReportResponse, error) {
	dep := dependency.FromContext(c)
	client := dep.AbuseReportClient()
	hasher := dep.HashIDEncoder()

	var (
		err            error
		category       int
		shareID        int
		reportedUserID int
		reporterID     int
	)

	if v := abuseCondition(s.Conditions, abuseProCategoryCondition, abuseCategoryCondition); v != "" {
		if category, err = strconv.Atoi(v); err != nil {
			return nil, serializer.NewError(serializer.CodeParamErr, "Invalid abuse category", err)
		}
	}
	if v := abuseCondition(s.Conditions, abuseProShareIDCondition, abuseShareIDCondition); v != "" {
		if shareID, err = strconv.Atoi(v); err != nil {
			var decodeErr error
			shareID, decodeErr = hasher.Decode(v, hashid.ShareID)
			if decodeErr != nil {
				return nil, serializer.NewError(serializer.CodeParamErr, "Invalid share ID", decodeErr)
			}
		}
	}
	if v := abuseCondition(s.Conditions, abuseProReportedUserCondition, abuseReportedUserCondition); v != "" {
		if reportedUserID, err = strconv.Atoi(v); err != nil {
			var decodeErr error
			reportedUserID, decodeErr = hasher.Decode(v, hashid.UserID)
			if decodeErr != nil {
				return nil, serializer.NewError(serializer.CodeParamErr, "Invalid reported user ID", decodeErr)
			}
		}
	}
	if v := abuseCondition(s.Conditions, abuseProReporterCondition, abuseReporterCondition); v != "" {
		if reporterID, err = strconv.Atoi(v); err != nil {
			var decodeErr error
			reporterID, decodeErr = hasher.Decode(v, hashid.UserID)
			if decodeErr != nil {
				return nil, serializer.NewError(serializer.CodeParamErr, "Invalid reporter ID", decodeErr)
			}
		}
	}

	ctx := context.WithValue(c, inventory.LoadAbuseReportRelation{}, true)
	res, err := client.List(ctx, &inventory.ListAbuseReportArgs{
		PaginationArgs: &inventory.PaginationArgs{
			Page:     s.Page - 1,
			PageSize: s.PageSize,
			Order:    inventory.OrderDirection(s.OrderDirection),
		},
		Category:       category,
		ShareID:        shareID,
		ReportedUserID: reportedUserID,
		ReporterID:     reporterID,
		FileURI:        abuseCondition(s.Conditions, abuseProFileURICondition, abuseFileURICondition),
	})
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to list abuse reports", err)
	}

	return &ListAbuseReportResponse{
		Pagination: res.PaginationResults,
		Reports: lo.Map(res.Reports, func(report *ent.AbuseReport, _ int) GetAbuseReportResponse {
			return buildAbuseReportResponse(report, hasher)
		}),
	}, nil
}

type (
	SingleAbuseReportService struct {
		ID int `uri:"id" binding:"required"`
	}
	SingleAbuseReportParamCtx struct{}
)

func (s *SingleAbuseReportService) Get(c *gin.Context) (*GetAbuseReportResponse, error) {
	dep := dependency.FromContext(c)
	client := dep.AbuseReportClient()
	hasher := dep.HashIDEncoder()

	ctx := context.WithValue(c, inventory.LoadAbuseReportRelation{}, true)
	report, err := client.GetByID(ctx, s.ID)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeDBError, "Failed to get abuse report", err)
	}

	res := buildAbuseReportResponse(report, hasher)
	return &res, nil
}

type (
	BatchAbuseReportService struct {
		IDs []int `json:"ids" binding:"required"`
	}
	BatchAbuseReportParamCtx struct{}
)

func (s *BatchAbuseReportService) Delete(c *gin.Context) error {
	dep := dependency.FromContext(c)
	if err := dep.AbuseReportClient().DeleteByIDs(c, s.IDs...); err != nil {
		return serializer.NewError(serializer.CodeDBError, "Failed to delete abuse reports", err)
	}

	return nil
}

func buildAbuseReportResponse(report *ent.AbuseReport, hasher hashid.Encoder) GetAbuseReportResponse {
	res := GetAbuseReportResponse{
		AbuseReport: report,
	}

	if report.Edges.Reporter != nil {
		res.ReporterHashID = hashid.EncodeUserID(hasher, report.Edges.Reporter.ID)
	}
	if report.Edges.ReportedUser != nil {
		res.ReportedUserHashID = hashid.EncodeUserID(hasher, report.Edges.ReportedUser.ID)
		res.UserHashID = res.ReportedUserHashID
	}
	if report.Edges.Share != nil {
		res.ShareHashID = hashid.EncodeShareID(hasher, report.Edges.Share.ID)
	}

	return res
}
