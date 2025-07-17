// internal/db/client.go
package db

import (
	"context"
	"database/sql"
	"log"

	"pollAppNew/ent"

	entsql "entgo.io/ent/dialect/sql"

	_ "github.com/jackc/pgx/v5/stdlib" // registers "pgx" with database/sql
)

func NewClient() *ent.Client {
	// 1) Open the DB using the pgx driver
	dbConn, err := sql.Open("pgx", "postgres://postgres:postgres@localhost:5432/pollappnew?sslmode=disable")
	if err != nil {
		log.Fatalf("failed opening db: %v", err)
	}
	// 2) Wrap it for Ent, telling it to use the "postgres" dialect
	drv := entsql.OpenDB("postgres", dbConn)
	client := ent.NewClient(ent.Driver(drv))

	//3) Run your schema migrations
	if err := client.Schema.Create(context.Background()); err != nil {
		log.Fatalf("failed running schema migration: %v", err)
	}
	return client
}
