package main

import (
	"context"
	"fmt"
	"log"

	"pollAppNew/ent"
)

func seed(ctx context.Context, client *ent.Client) {
	// 1️⃣ Create 10 users
	users := make([]*ent.User, 0, 10)
	for i := 1; i <= 10; i++ {
		u, err := client.User.
			Create().
			SetUsername(fmt.Sprintf("user%02d", i)).
			SetPasswordHash("pass123").
			Save(ctx)
		if err != nil {
			log.Fatalf("failed creating user %d: %v", i, err)
		}
		users = append(users, u)
	}

	// 2️⃣ Create 5 polls, each by a different creator, with 2–3 options
	polls := make([]*ent.Poll, 0, 5)
	for i := 1; i <= 5; i++ {
		creator := users[(i-1)%len(users)]
		p, err := client.Poll.
			Create().
			SetTitle(fmt.Sprintf("Poll %d", i)).
			SetCreator(creator).
			Save(ctx)
		if err != nil {
			log.Fatalf("failed creating poll %d: %v", i, err)
		}
		polls = append(polls, p)

		// Decide 2 or 3 options
		numOpts := 2
		if i%2 == 0 {
			numOpts = 3
		}
		for j := 1; j <= numOpts; j++ {
			if _, err := client.PollOption.
				Create().
				SetText(fmt.Sprintf("Option %d for Poll %d", j, i)).
				SetPoll(p).
				Save(ctx); err != nil {
				log.Fatalf("failed creating option %d for poll %d: %v", j, i, err)
			}
		}
	}

	// 3️⃣ Cast votes: first 3 polls get 3 voters; last 2 polls get 2 voters
	for idx, p := range polls {
		opts, err := p.QueryOptions().All(ctx)
		if err != nil {
			log.Fatalf("failed querying options for poll %d: %v", p.ID, err)
		}

		// choose voters slice
		var voters []*ent.User
		if idx < 3 {
			voters = users[:3]
		} else {
			voters = users[:2]
		}

		for _, u := range voters {
			// for simplicity vote always for the first option
			if _, err := client.Vote.
				Create().
				SetUser(u).
				SetPoll(p).
				SetOption(opts[0]).
				Save(ctx); err != nil {
				log.Fatalf("failed creating vote for user %s on poll %d: %v", u.Username, p.ID, err)
			}
		}
	}

	log.Println("✅ Database seeded: 10 users, 5 polls, options & votes created.")
}
