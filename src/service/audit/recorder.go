package audit

import (
	"context"

	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/cloudreve/Cloudreve/v4/pkg/auth/requestinfo"
	"github.com/cloudreve/Cloudreve/v4/pkg/logging"
	"github.com/gofrs/uuid"
)

// Record writes an audit log entry.
//
// Failures are logged and swallowed: auditing must never break the operation
// being audited.
func Record(ctx context.Context, t EventType, entry *LogEntry, opts ...Option) {
	dep, ok := ctx.Value(dependency.DepCtx{}).(dependency.Dep)
	if !ok || dep == nil {
		return
	}

	settings := dep.SettingProvider()
	enabled := settings.AuditEnabled(ctx, int(t))
	recordIP := settings.AuditRecordIP(ctx)
	if !enabled {
		return
	}

	args := newRecordOptions()
	for _, opt := range opts {
		opt(args)
	}

	content := &types.AuditLogEntry{}
	if entry != nil {
		content = &types.AuditLogEntry{
			Category:          int(categoryOf(t)),
			Failed:            entry.Failed,
			Error:             entry.Error,
			UserAgent:         entry.UserAgent,
			IsSystem:          entry.IsSystem,
			Reason:            entry.Reason,
			EmailTo:           entry.EmailTo,
			EmailTitle:        entry.EmailTitle,
			OriginalName:      entry.OriginalName,
			NewName:           entry.NewName,
			From:              entry.From,
			To:                entry.To,
			EntityCreateTime:  entry.EntityCreateTime,
			StoragePolicyID:   entry.StoragePolicyID,
			StoragePolicyName: entry.StoragePolicyName,
			AccountID:         entry.AccountID,
			Account:           entry.Account,
			AccountURI:        entry.AccountURI,
			PaymentID:         entry.PaymentID,
			PointsChange:      entry.PointsChange,
			SKU:               entry.SKU,
			StorageSize:       entry.StorageSize,
			Expire:            entry.Expire,
			GroupID:           entry.GroupID,
			GroupIDFrom:       entry.GroupIDFrom,
			DirectLinkID:      entry.DirectLinkID,
			OpenIDProvider:    entry.OpenIDProvider,
			Sub:               entry.Sub,
			Name:              entry.Name,
			PasskeyID:         entry.PasskeyID,
			LoginWith:         entry.LoginWith,
			Passkey:           entry.Passkey,
			Exts:              entry.Exts,
		}
	}

	record := &inventory.AuditLog{
		Type:    int(t),
		Content: content,
	}

	if info := requestinfo.RequestInfoFromContext(ctx); info != nil && recordIP {
		record.IP = info.IP
		if content.UserAgent == "" {
			content.UserAgent = info.UserAgent
		}
	}

	if user := inventory.UserFromContext(ctx); user != nil && user.ID > 0 {
		record.UserID = user.ID
	}

	record.UserID = firstNonZero(record.UserID, args.userID)
	record.FileID = args.fileID
	record.EntityID = args.entityID
	record.ShareID = args.shareID
	record.CorrelationID = args.correlationID
	if record.CorrelationID == nil {
		// Default to the current request/job correlation ID so audit logs can
		// be traced back to the originating API request or task.
		if cid := logging.CorrelationID(ctx); cid != uuid.Nil {
			record.CorrelationID = &cid
		}
	}

	if _, err := dep.AuditLogClient().Create(ctx, record); err != nil {
		logging.FromContext(ctx).Warning("Failed to write audit log (type %d): %s", t, err)
	}
}

// Option customizes an audit record.
type Option func(*recordOptions)

type recordOptions struct {
	userID        int
	fileID        int
	entityID      int
	shareID       int
	correlationID *uuid.UUID
}

func newRecordOptions() *recordOptions {
	return &recordOptions{}
}

// WithUser overrides the user ID recorded in the log.
func WithUser(id int) Option {
	return func(o *recordOptions) { o.userID = id }
}

// WithFile attaches a file to the log.
func WithFile(id int) Option {
	return func(o *recordOptions) { o.fileID = id }
}

// WithEntity attaches an entity to the log.
func WithEntity(id int) Option {
	return func(o *recordOptions) { o.entityID = id }
}

// WithShare attaches a share to the log.
func WithShare(id int) Option {
	return func(o *recordOptions) { o.shareID = id }
}

// WithCorrelationID attaches a correlation ID to the log.
func WithCorrelationID(id uuid.UUID) Option {
	return func(o *recordOptions) { o.correlationID = &id }
}

func firstNonZero(values ...int) int {
	for _, v := range values {
		if v != 0 {
			return v
		}
	}
	return 0
}
