// internal/db/client.go
package db

import (
	"context"
	"database/sql"
	"flag"
	"log"

	"pollAppNew/ent"

	entsql "entgo.io/ent/dialect/sql"

	_ "github.com/jackc/pgx/v5/stdlib" // registers "pgx" with database/sql
)

var (
	// Declare the -db flag here. Calm’s macro will expand @@{PostgresService.address}@@ into the real IP.
	dbConnStr = flag.String("db", "", "Postgres connection string")
)

func NewClient() *ent.Client {
	// 1) Open the DB using the pgx driver
	if *dbConnStr == "" {
		log.Fatal("must pass -db <connection-string>, e.g. -db \"postgres://user:pass@IP:5432/dbname?sslmode=disable\"")
	}
	dbConn, err := sql.Open("pgx", *dbConnStr)
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
