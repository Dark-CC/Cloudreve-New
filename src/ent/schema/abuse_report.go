package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// AbuseReport holds the schema definition for the AbuseReport entity.
type AbuseReport struct {
	ent.Schema
}

// Fields of the AbuseReport.
func (AbuseReport) Fields() []ent.Field {
	return []ent.Field{
		field.String("file_uri").
			Optional(),
		field.Int("category").
			Default(0),
		field.String("description").
			Optional(),
		field.Int("share_id").
			Optional().
			Nillable(),
		field.Int("reported_user_id").
			Optional().
			Nillable(),
		field.Int("reporter_id").
			Optional().
			Nillable(),
	}
}

// Edges of the AbuseReport.
func (AbuseReport) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("share", Share.Type).
			Ref("abuse_reports").
			Field("share_id").
			Unique().
			Annotations(entsql.OnDelete(entsql.SetNull)),
		edge.From("reported_user", User.Type).
			Ref("abuse_reports_as_reported").
			Field("reported_user_id").
			Unique().
			Annotations(entsql.OnDelete(entsql.SetNull)),
		edge.From("reporter", User.Type).
			Ref("abuse_reports_as_reporter").
			Field("reporter_id").
			Unique().
			Annotations(entsql.OnDelete(entsql.SetNull)),
	}
}

func (AbuseReport) Mixin() []ent.Mixin {
	return []ent.Mixin{
		CommonMixin{},
	}
}
