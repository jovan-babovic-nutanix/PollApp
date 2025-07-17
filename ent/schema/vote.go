package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Vote holds the schema definition for the Vote entity.
type Vote struct {
	ent.Schema
}

// Fields defines the FK columns in snake_case.
func (Vote) Fields() []ent.Field {
	return []ent.Field{
		field.Int("user_id"),
		field.Int("poll_id"),
		field.Int("option_id"),
	}
}

// Edges wires up the relationships and points them at those exact fields.
func (Vote) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("votes").
			Field("user_id").
			Unique().
			Required(),

		edge.From("poll", Poll.Type).
			Ref("votes").
			Field("poll_id").
			Unique().
			Required(),

		// If your schema type is Option, use Option.Type.
		// If you actually called your schema PollOption, keep PollOption.Type.
		edge.From("option", PollOption.Type).
			Ref("votes").
			Field("option_id").
			Unique().
			Required(),
	}
}

// Composite index to enforce one vote per user per poll.
func (Vote) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "poll_id").
			Unique(),
	}
}
