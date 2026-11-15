package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/cloudreve/Cloudreve/v4/inventory/types"
	"github.com/gofrs/uuid"
)

// AuditLog holds the schema definition for the AuditLog entity.
type AuditLog struct {
	ent.Schema
}

// Fields of the AuditLog.
func (AuditLog) Fields() []ent.Field {
	return []ent.Field{
		field.Int("type"),
		field.UUID("correlation_id", uuid.Nil).
			Optional().
			Nillable(),
		field.String("ip").
			Optional(),
		field.JSON("content", &types.AuditLogEntry{}).
			Optional(),
		field.Int("entity_audit_logs").
			Optional().
			Nillable(),
		field.Int("file_audit_logs").
			Optional().
			Nillable(),
		field.Int("share_audit_logs").
			Optional().
			Nillable(),
		field.Int("user_audit_logs").
			Optional().
			Nillable(),
	}
}

// Edges of the AuditLog.
func (AuditLog) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("entity", Entity.Type).
			Ref("audit_logs").
			Field("entity_audit_logs").
			Unique().
			Annotations(entsql.OnDelete(entsql.SetNull)),
		edge.From("file", File.Type).
			Ref("audit_logs").
			Field("file_audit_logs").
			Unique().
			Annotations(entsql.OnDelete(entsql.SetNull)),
		edge.From("share", Share.Type).
			Ref("audit_logs").
			Field("share_audit_logs").
			Unique().
			Annotations(entsql.OnDelete(entsql.SetNull)),
		edge.From("user", User.Type).
			Ref("audit_logs").
			Field("user_audit_logs").
			Unique().
			Annotations(entsql.OnDelete(entsql.SetNull)),
	}
}

func (AuditLog) Mixin() []ent.Mixin {
	return []ent.Mixin{
		CommonMixin{},
	}
}
