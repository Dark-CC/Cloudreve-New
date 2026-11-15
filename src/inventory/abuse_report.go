package inventory

import (
	"context"

	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/abusereport"
	"github.com/cloudreve/Cloudreve/v4/pkg/conf"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
)

type (
	// LoadAbuseReportRelation eager loads related user and share.
	LoadAbuseReportRelation struct{}

	AbuseReportClient interface {
		TxOperator
		// Create creates a new abuse report.
		Create(ctx context.Context, report *ent.AbuseReport) (*ent.AbuseReport, error)
		// List lists abuse reports with pagination.
		List(ctx context.Context, args *ListAbuseReportArgs) (*ListAbuseReportResult, error)
		// GetByID returns an abuse report by ID.
		GetByID(ctx context.Context, id int) (*ent.AbuseReport, error)
		// DeleteByIDs soft deletes abuse reports by IDs.
		DeleteByIDs(ctx context.Context, ids ...int) error
		// CountByReporter returns the number of reports created by given user.
		CountByReporter(ctx context.Context, reporterID int) (int, error)
	}

	ListAbuseReportArgs struct {
		*PaginationArgs
		Category       int
		ShareID        int
		ReportedUserID int
		ReporterID     int
		FileURI        string
	}

	ListAbuseReportResult struct {
		*PaginationResults
		Reports []*ent.AbuseReport
	}
)

func NewAbuseReportClient(client *ent.Client, dbType conf.DBType) AbuseReportClient {
	return &abuseReportClient{client: client, maxSQlParam: sqlParamLimit(dbType)}
}

type abuseReportClient struct {
	client      *ent.Client
	maxSQlParam int
}

func (c *abuseReportClient) SetClient(newClient *ent.Client) TxOperator {
	return &abuseReportClient{client: newClient, maxSQlParam: c.maxSQlParam}
}

func (c *abuseReportClient) GetClient() *ent.Client {
	return c.client
}

func (c *abuseReportClient) Create(ctx context.Context, report *ent.AbuseReport) (*ent.AbuseReport, error) {
	if report.ShareID == nil && report.ReportedUserID == nil {
		return nil, serializer.NewError(serializer.CodeParamErr, "Either share or reported user is required", nil)
	}

	stm := c.client.AbuseReport.Create().
		SetCategory(report.Category).
		SetNillableShareID(report.ShareID).
		SetNillableReportedUserID(report.ReportedUserID).
		SetNillableReporterID(report.ReporterID)

	if report.FileURI != "" {
		stm.SetFileURI(report.FileURI)
	}
	if report.Description != "" {
		stm.SetDescription(report.Description)
	}

	return stm.Save(ctx)
}

func (c *abuseReportClient) List(ctx context.Context, args *ListAbuseReportArgs) (*ListAbuseReportResult, error) {
	query := withAbuseReportEagerLoading(ctx, c.client.AbuseReport.Query())
	if args.Category > 0 {
		query.Where(abusereport.CategoryEQ(args.Category))
	}
	if args.ShareID > 0 {
		query.Where(abusereport.ShareID(args.ShareID))
	}
	if args.ReportedUserID > 0 {
		query.Where(abusereport.ReportedUserID(args.ReportedUserID))
	}
	if args.ReporterID > 0 {
		query.Where(abusereport.ReporterID(args.ReporterID))
	}
	if args.FileURI != "" {
		query.Where(abusereport.FileURIContains(args.FileURI))
	}

	orderOpt := abusereport.ByID(getOrderTerm(args.Order))
	if args.OrderBy == "created_at" {
		orderOpt = abusereport.ByCreatedAt(getOrderTerm(args.Order))
	}

	pageSize := capPageSize(c.maxSQlParam, args.PageSize, 10)
	total, err := query.Clone().Count(ctx)
	if err != nil {
		return nil, err
	}

	reports, err := query.
		Order(orderOpt).
		Limit(pageSize).
		Offset(args.Page * pageSize).
		All(ctx)
	if err != nil {
		return nil, err
	}

	return &ListAbuseReportResult{
		Reports: reports,
		PaginationResults: &PaginationResults{
			TotalItems: total,
			Page:       args.Page,
			PageSize:   pageSize,
		},
	}, nil
}

func (c *abuseReportClient) GetByID(ctx context.Context, id int) (*ent.AbuseReport, error) {
	return withAbuseReportEagerLoading(ctx, c.client.AbuseReport.Query()).Where(abusereport.ID(id)).First(ctx)
}

func (c *abuseReportClient) DeleteByIDs(ctx context.Context, ids ...int) error {
	if len(ids) == 0 {
		return serializer.NewError(serializer.CodeParamErr, "No abuse report IDs specified", nil)
	}

	if len(ids) > c.maxSQlParam-10 {
		return ErrTooManyArguments
	}

	_, err := c.client.AbuseReport.Delete().Where(abusereport.IDIn(ids...)).Exec(ctx)
	return err
}

func (c *abuseReportClient) CountByReporter(ctx context.Context, reporterID int) (int, error) {
	return c.client.AbuseReport.Query().Where(abusereport.ReporterID(reporterID)).Count(ctx)
}

func withAbuseReportEagerLoading(ctx context.Context, q *ent.AbuseReportQuery) *ent.AbuseReportQuery {
	if _, ok := ctx.Value(LoadAbuseReportRelation{}).(bool); ok {
		q.WithShare().WithReportedUser().WithReporter()
	}
	return q
}
