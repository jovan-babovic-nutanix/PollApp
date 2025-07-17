package main

import (
	"context"
	"log"
	"net/http"

	"pollAppNew/internal/db"
	"pollAppNew/router"
)

func main() {
	client := db.NewClient()
	defer client.Close()

	ctx := context.Background()
	userCount, err := client.User.
		Query().
		Count(ctx)
	if err != nil {
		log.Fatalf("failed counting users: %v", err)
	}
	if userCount == 0 {
		log.Println("seeding database with initial data…")
		seed(ctx, client)
	} else {
		log.Printf("skipping seed; %d users already exist\n", userCount)
	}

	r := router.Setup(client)
	log.Println("Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}
