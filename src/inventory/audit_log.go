package inventory

import (
	"context"
	"fmt"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/auditlog"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/conf"
	"github.com/cloudreve/Cloudreve/v4/pkg/hashid"
	"github.com/gofrs/uuid"
)

type (
	// LoadAuditLogRelation eager loads related user/file/entity/share.
	LoadAuditLogRelation struct{}

	AuditLogClient interface {
		TxOperator
		// Create records an audit log entry.
		Create(ctx context.Context, log *AuditLog) (*ent.AuditLog, error)
		// GetByID returns one audit log entry by primary key.
		GetByID(ctx context.Context, id int) (*ent.AuditLog, error)
		// List returns audit logs with pagination.
		List(ctx context.Context, args *ListAuditLogArgs) (*ListAuditLogResult, error)
		// ListFileActivities returns file-scoped audit logs with cursor pagination.
		ListFileActivities(ctx context.Context, args *ListFileActivityArgs) (*ListAuditLogResult, error)
		// DeleteBefore removes logs older than the given time.
		DeleteBefore(ctx context.Context, before time.Time) (int, error)
		// Archive returns every log matching the range and categories without pagination.
		Archive(ctx context.Context, since, until *time.Time, categories []int) ([]*ent.AuditLog, error)
		// DeleteByRange removes logs matching the range and categories.
		DeleteByRange(ctx context.Context, since, until *time.Time, categories []int) (int, error)
		// DeleteByIDs removes logs by primary key.
		DeleteByIDs(ctx context.Context, ids []int) (int, error)
	}

	// AuditLog is the input for a new audit log record.
	AuditLog struct {
		Type          int
		IP            string
		CorrelationID *uuid.UUID
		Content       *types.AuditLogEntry
		UserID        int
		FileID        int
		EntityID      int
		ShareID       int
	}

	ListAuditLogArgs struct {
		*PaginationArgs
		Types         []int
		UserID        int
		FileID        int
		IP            string
		CorrelationID *uuid.UUID
		Since         *time.Time
		Until         *time.Time
	}

	ListAuditLogResult struct {
		*PaginationResults
		Logs []*ent.AuditLog
	}

	ListFileActivityArgs struct {
		FileID         int
		UserID         int
		Types          []int
		PageSize       int
		OrderDirection OrderDirection
		PageToken      string
	}
)

func NewAuditLogClient(client *ent.Client, dbType conf.DBType, hasher hashid.Encoder) AuditLogClient {
	return &auditLogClient{client: client, maxSQlParam: sqlParamLimit(dbType), hasher: hasher}
}

type auditLogClient struct {
	client      *ent.Client
	maxSQlParam int
	hasher      hashid.Encoder
}

func (c *auditLogClient) SetClient(newClient *ent.Client) TxOperator {
	return &auditLogClient{client: newClient, maxSQlParam: c.maxSQlParam, hasher: c.hasher}
}

func (c *auditLogClient) GetClient() *ent.Client {
	return c.client
}

func (c *auditLogClient) Create(ctx context.Context, log *AuditLog) (*ent.AuditLog, error) {
	stm := c.client.AuditLog.Create().SetType(log.Type)

	if log.IP != "" {
		stm.SetIP(log.IP)
	}
	if log.CorrelationID != nil {
		stm.SetCorrelationID(*log.CorrelationID)
	}
	if log.Content != nil {
		stm.SetContent(log.Content)
	}
	if log.UserID > 0 {
		stm.SetUserAuditLogs(log.UserID)
	}
	if log.FileID > 0 {
		stm.SetFileAuditLogs(log.FileID)
	}
	if log.EntityID > 0 {
		stm.SetEntityAuditLogs(log.EntityID)
	}
	if log.ShareID > 0 {
		stm.SetShareAuditLogs(log.ShareID)
	}

	return stm.Save(ctx)
}

func (c *auditLogClient) GetByID(ctx context.Context, id int) (*ent.AuditLog, error) {
	return withAuditLogEagerLoading(ctx, c.client.AuditLog.Query()).Where(auditlog.ID(id)).First(ctx)
}

func (c *auditLogClient) List(ctx context.Context, args *ListAuditLogArgs) (*ListAuditLogResult, error) {
	query := withAuditLogEagerLoading(ctx, c.client.AuditLog.Query())
	if len(args.Types) > 0 {
		query.Where(auditlog.TypeIn(args.Types...))
	}
	if args.UserID > 0 {
		query.Where(auditlog.UserAuditLogs(args.UserID))
	}
	if args.FileID > 0 {
		query.Where(auditlog.FileAuditLogs(args.FileID))
	}
	if args.IP != "" {
		query.Where(auditlog.IP(args.IP))
	}
	if args.CorrelationID != nil {
		query.Where(auditlog.CorrelationID(*args.CorrelationID))
	}
	if args.Since != nil {
		query.Where(auditlog.CreatedAtGTE(normalizeAuditLogTime(*args.Since)))
	}
	if args.Until != nil {
		query.Where(auditlog.CreatedAtLTE(normalizeAuditLogTime(*args.Until)))
	}

	pageSize := capPageSize(c.maxSQlParam, args.PageSize, 10)
	total, err := query.Clone().Count(ctx)
	if err != nil {
		return nil, err
	}

	logs, err := query.
		Order(auditlog.ByID(getOrderTerm(args.Order))).
		Limit(pageSize).
		Offset(args.Page * pageSize).
		All(ctx)
	if err != nil {
		return nil, err
	}

	return &ListAuditLogResult{
		Logs: logs,
		PaginationResults: &PaginationResults{
			TotalItems: total,
			Page:       args.Page,
			PageSize:   pageSize,
		},
	}, nil
}

func (c *auditLogClient) ListFileActivities(ctx context.Context, args *ListFileActivityArgs) (*ListAuditLogResult, error) {
	query := withAuditLogEagerLoading(ctx, c.client.AuditLog.Query()).
		Where(auditlog.FileAuditLogs(args.FileID))
	if args.UserID > 0 {
		query.Where(auditlog.UserAuditLogs(args.UserID))
	}
	if len(args.Types) > 0 {
		query.Where(auditlog.TypeIn(args.Types...))
	}

	pageSize := args.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	order := sql.OrderDesc()
	if args.OrderDirection == OrderDirectionAsc {
		order = sql.OrderAsc()
	}
	query.Order(auditlog.ByID(order))

	if args.PageToken != "" {
		token, err := pageTokenFromString(args.PageToken, c.hasher, hashid.AuditLogID)
		if err != nil {
			return nil, fmt.Errorf("invalid page token %q: %w", args.PageToken, err)
		}
		if args.OrderDirection == OrderDirectionAsc {
			query.Where(auditlog.IDGT(token.ID))
		} else {
			query.Where(auditlog.IDLT(token.ID))
		}
	}

	logs, err := query.Limit(pageSize + 1).All(ctx)
	if err != nil {
		return nil, err
	}

	hasMore := len(logs) > pageSize
	if hasMore {
		logs = logs[:pageSize]
	}

	nextToken := ""
	if hasMore && len(logs) > 0 {
		last := logs[len(logs)-1]
		token := &PageToken{ID: last.ID}
		nextToken, err = token.Encode(c.hasher, hashid.EncodeAuditLogID)
		if err != nil {
			return nil, fmt.Errorf("failed to generate next page token: %w", err)
		}
	}

	return &ListAuditLogResult{
		Logs: logs,
		PaginationResults: &PaginationResults{
			PageSize:      pageSize,
			NextPageToken: nextToken,
			IsCursor:      true,
		},
	}, nil
}

func (c *auditLogClient) DeleteBefore(ctx context.Context, before time.Time) (int, error) {
	return c.client.AuditLog.Delete().
		Where(auditlog.CreatedAtLT(normalizeAuditLogTime(before))).
		Exec(ctx)
}

func (c *auditLogClient) archiveQuery(ctx context.Context, since, until *time.Time, categories []int) *ent.AuditLogQuery {
	query := c.client.AuditLog.Query()
	if len(categories) > 0 {
		query.Where(auditlog.TypeIn(categories...))
	}
	if since != nil {
		query.Where(auditlog.CreatedAtGTE(normalizeAuditLogTime(*since)))
	}
	if until != nil {
		query.Where(auditlog.CreatedAtLTE(normalizeAuditLogTime(*until)))
	}
	return query
}

func (c *auditLogClient) Archive(ctx context.Context, since, until *time.Time, categories []int) ([]*ent.AuditLog, error) {
	return c.archiveQuery(ctx, since, until, categories).Order(auditlog.ByID()).All(ctx)
}

func (c *auditLogClient) DeleteByRange(ctx context.Context, since, until *time.Time, categories []int) (int, error) {
	query := c.client.AuditLog.Delete()
	if len(categories) > 0 {
		query.Where(auditlog.TypeIn(categories...))
	}
	if since != nil {
		query.Where(auditlog.CreatedAtGTE(normalizeAuditLogTime(*since)))
	}
	if until != nil {
		query.Where(auditlog.CreatedAtLTE(normalizeAuditLogTime(*until)))
	}
	return query.Exec(ctx)
}

func (c *auditLogClient) DeleteByIDs(ctx context.Context, ids []int) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	return c.client.AuditLog.Delete().Where(auditlog.IDIn(ids...)).Exec(ctx)
}

// AuditCategoriesToTypes maps public category values to the concrete event
// types stored on audit log rows. Values that are already event types are
// preserved so internal callers keep working.
func AuditCategoriesToTypes(categories []int) []int {
	eventTypesByCategory := map[int][]int{
		0: {0},
		1: {1, 3, 4, 5, 6, 41, 42, 44, 45, 46, 47, 49, 50, 51, 52, 53, 58, 59, 60, 61},
		2: {7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 23, 43, 55, 56, 57},
		3: {17, 18, 24, 25},
		4: {19, 20},
		5: {21, 22},
		6: {26, 27, 28, 29},
		7: {30, 31, 32, 33},
		8: {34, 35, 36, 37, 38, 39, 40, 48, 54},
		9: {2},
	}

	res := make([]int, 0)
	seen := make(map[int]struct{})
	for _, category := range categories {
		if category < 0 {
			continue
		}
		mapped, ok := eventTypesByCategory[category]
		if !ok {
			if _, exists := seen[category]; !exists {
				seen[category] = struct{}{}
				res = append(res, category)
			}
			continue
		}
		for _, eventType := range mapped {
			if _, exists := seen[eventType]; exists {
				continue
			}
			seen[eventType] = struct{}{}
			res = append(res, eventType)
		}
	}
	return res
}

func auditCategoriesToTypes(categories []int) []int {
	return AuditCategoriesToTypes(categories)
}

// normalizeAuditLogTime converts an API time bound to the server's local time
// zone. SQLite stores the default timestamps as time.Time.String(), which
// includes the local numeric offset. Comparing that text against a UTC-bound
// parameter makes the bounds miss every row, so normalize before querying.
func normalizeAuditLogTime(t time.Time) time.Time {
	return t.In(time.Local)
}

func withAuditLogEagerLoading(ctx context.Context, q *ent.AuditLogQuery) *ent.AuditLogQuery {
	if _, ok := ctx.Value(LoadAuditLogRelation{}).(bool); ok {
		q.WithUser().WithFile().WithEntity().WithShare()
	}
	return q
}
