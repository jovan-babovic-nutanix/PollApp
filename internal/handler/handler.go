package handler

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"pollAppNew/ent"
	"pollAppNew/ent/poll"
	"pollAppNew/ent/polloption"
	"pollAppNew/ent/user"
	"pollAppNew/ent/vote"
	"strconv"
	"time"

	"github.com/jackc/pgconn"
	"github.com/julienschmidt/httprouter"
)

// SignUp handles user registration (raw password, not for production).
func SignUp(client *ent.Client) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		ctx := r.Context()

		// 1) Decode JSON body
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON payload", http.StatusBadRequest)
			return
		}
		if req.Username == "" || req.Password == "" {
			http.Error(w, "username and password required", http.StatusBadRequest)
			return
		}

		// 2) Create user with raw password
		u, err := client.User.
			Create().
			SetUsername(req.Username).
			SetPasswordHash(req.Password).
			Save(ctx)
		if err != nil {
			http.Error(w, "could not create user", http.StatusInternalServerError)
			return
		}

		// 3) Return new user (ID and username only)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"user_id":  u.ID,
			"username": u.Username,
		})
	}
}

// Login handles user authentication (raw passwords, not for production).
func Login(client *ent.Client) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		ctx := r.Context()
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON payload", http.StatusBadRequest)
			return
		}

		// fetch user by username
		u, err := client.User.
			Query().
			Where(user.UsernameEQ(req.Username)).
			Only(ctx)
		if err != nil {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}

		// compare raw passwords
		if req.Password != u.PasswordHash {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}

		// set a cookie with the user ID
		cookie := http.Cookie{
			Name:     "user_id",
			Value:    strconv.Itoa(u.ID),
			Path:     "/",
			HttpOnly: true,
			Expires:  time.Now().Add(24 * time.Hour),
		}
		http.SetCookie(w, &cookie)

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message":"login successful"}`))
	}
}

// CreatePoll handles creating a new poll, using the logged-in user from a cookie.
func CreatePoll(client *ent.Client) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		ctx := r.Context()

		// 0) Authenticate via cookie
		c, err := r.Cookie("user_id")
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		userID, err := strconv.Atoi(c.Value)
		if err != nil {
			http.Error(w, "invalid user_id cookie", http.StatusUnauthorized)
			return
		}

		// 1) Decode request (no creator_id field)
		var req struct {
			Title   string   `json:"title"`
			Options []string `json:"options"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON payload", http.StatusBadRequest)
			return
		}
		if req.Title == "" {
			http.Error(w, "title is required", http.StatusBadRequest)
			return
		}
		if len(req.Options) < 2 {
			http.Error(w, "at least two options are required", http.StatusBadRequest)
			return
		}

		// 2) Begin transaction
		tx, err := client.Tx(ctx)
		if err != nil {
			log.Printf("failed to start tx: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		rollback := func() {
			if rbErr := tx.Rollback(); rbErr != nil {
				log.Printf("tx rollback error: %v", rbErr)
			}
		}

		// 3) Create the Poll with cookie userID
		p, err := tx.Poll.
			Create().
			SetTitle(req.Title).
			SetCreatorID(userID).
			Save(ctx)
		if err != nil {
			rollback()
			log.Printf("failed creating poll: %v", err)
			http.Error(w, "could not create poll", http.StatusInternalServerError)
			return
		}

		// 4) Create each PollOption
		createdOpts := make([]*ent.PollOption, 0, len(req.Options))
		for _, text := range req.Options {
			o, err := tx.PollOption.
				Create().
				SetText(text).
				SetPoll(p).
				Save(ctx)
			if err != nil {
				rollback()
				log.Printf("failed creating option %q: %v", text, err)
				http.Error(w, "could not create poll options", http.StatusInternalServerError)
				return
			}
			createdOpts = append(createdOpts, o)
		}

		// 5) Commit the transaction
		if err := tx.Commit(); err != nil {
			rollback()
			log.Printf("failed committing tx: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		// 6) Build and return response
		type optionResp struct {
			ID   int    `json:"id"`
			Text string `json:"text"`
		}
		type pollResp struct {
			ID        int          `json:"id"`
			Title     string       `json:"title"`
			CreatorID int          `json:"creator_id"`
			Options   []optionResp `json:"options"`
		}

		opts := make([]optionResp, len(createdOpts))
		for i, o := range createdOpts {
			opts[i] = optionResp{ID: o.ID, Text: o.Text}
		}

		resp := pollResp{
			ID:        p.ID,
			Title:     p.Title,
			CreatorID: p.CreatorID,
			Options:   opts,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Printf("failed encoding response: %v", err)
		}
	}
}

// GetPoll retrieves a poll by ID.
func GetPoll(client *ent.Client) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		ctx := r.Context()

		// 1) Parse & validate `id` param
		idStr := ps.ByName("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "invalid poll id", http.StatusBadRequest)
			return
		}

		// 2) Load poll with its options + votes
		p, err := client.Poll.
			Query().
			Where(poll.IDEQ(id)).
			WithOptions(func(oq *ent.PollOptionQuery) {
				oq.WithVotes()
			}).
			Only(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				http.Error(w, "poll not found", http.StatusNotFound)
			} else {
				log.Printf("failed querying poll: %v", err)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
			return
		}

		// 3) Build response structs
		type optionResponse struct {
			ID    int    `json:"id"`
			Text  string `json:"text"`
			Votes int    `json:"votes"`
		}
		type pollResponse struct {
			ID        int              `json:"id"`
			Title     string           `json:"title"`
			CreatorID int              `json:"creator_id"`
			Options   []optionResponse `json:"options"`
		}

		opts := make([]optionResponse, len(p.Edges.Options))
		for i, o := range p.Edges.Options {
			opts[i] = optionResponse{
				ID:    o.ID,
				Text:  o.Text,
				Votes: len(o.Edges.Votes),
			}
		}

		resp := pollResponse{
			ID:        p.ID,
			Title:     p.Title,
			CreatorID: p.CreatorID,
			Options:   opts,
		}

		// 4) JSON-encode and return
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Printf("failed encoding response: %v", err)
			http.Error(w, "failed encoding response", http.StatusInternalServerError)
		}
	}
}

// ListPolls retrieves all polls
func ListPolls(client *ent.Client) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		ctx := r.Context()

		// 1. Query all polls, eager‐loading their options:
		polls, err := client.Poll.
			Query().
			WithOptions().
			All(ctx)
		if err != nil {
			http.Error(w, "failed querying polls", http.StatusInternalServerError)
			// log the actual DB error for debugging:
			log.Printf("ERROR querying polls: %v\n", err)
			http.Error(w, "internal error querying polls", http.StatusInternalServerError)
			return
		}

		// 2. Serialize to JSON and send:
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(polls); err != nil {
			http.Error(w, "failed encoding response", http.StatusInternalServerError)
			return
		}
	}
}

// Vote handles voting on an option in a poll and returns updated results.
func Vote(client *ent.Client) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		ctx := r.Context()

		// 0) Authenticate via cookie
		c, err := r.Cookie("user_id")
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		userID, err := strconv.Atoi(c.Value)
		if err != nil {
			http.Error(w, "invalid user_id cookie", http.StatusUnauthorized)
			return
		}

		// 0b) Verify user still exists
		exists, err := client.User.
			Query().
			Where(user.IDEQ(userID)).
			Exist(ctx)
		if err != nil {
			log.Printf("error checking user existence: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if !exists {
			// clear bad cookie
			http.SetCookie(w, &http.Cookie{
				Name:     "user_id",
				Value:    "",
				Path:     "/",
				HttpOnly: true,
				MaxAge:   -1,
			})
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// 1) Parse poll ID from path
		pollID, err := strconv.Atoi(ps.ByName("id"))
		if err != nil {
			http.Error(w, "invalid poll id", http.StatusBadRequest)
			return
		}

		// 2) Decode request body
		var req struct {
			OptionID int `json:"option_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON payload", http.StatusBadRequest)
			return
		}

		// 3) Prevent duplicate vote
		voted, err := client.Vote.
			Query().
			Where(vote.UserIDEQ(userID), vote.PollIDEQ(pollID)).
			Exist(ctx)
		if err != nil {
			log.Printf("error checking existing vote: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if voted {
			http.Error(w, "user has already voted on this poll", http.StatusConflict)
			return
		}

		// 4) Insert the vote
		if _, err = client.Vote.
			Create().
			SetUserID(userID).
			SetPollID(pollID).
			SetOptionID(req.OptionID).
			Save(ctx); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23503" {
				switch pgErr.ConstraintName {
				case "votes_poll_id_fkey":
					http.Error(w, "invalid poll id", http.StatusBadRequest)
				case "votes_option_id_fkey":
					http.Error(w, "invalid option id", http.StatusBadRequest)
				case "votes_user_id_fkey":
					http.Error(w, "unauthorized", http.StatusUnauthorized)
				default:
					http.Error(w, "invalid vote parameters", http.StatusBadRequest)
				}
				return
			}
			log.Printf("failed creating vote: %v", err)
			http.Error(w, "could not cast vote", http.StatusInternalServerError)
			return
		}

		// 5) Load updated results
		opts, err := client.PollOption.
			Query().
			Where(polloption.PollIDEQ(pollID)).
			WithVotes().
			All(ctx)
		if err != nil {
			log.Printf("error querying results: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		// 6) Build results response
		type result struct {
			OptionID int    `json:"option_id"`
			Text     string `json:"text"`
			Votes    int    `json:"votes"`
		}
		results := make([]result, len(opts))
		for i, o := range opts {
			results[i] = result{
				OptionID: o.ID,
				Text:     o.Text,
				Votes:    len(o.Edges.Votes),
			}
		}

		resp := struct {
			PollID  int      `json:"poll_id"`
			Results []result `json:"results"`
		}{
			PollID:  pollID,
			Results: results,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Printf("failed encoding response: %v", err)
		}
	}
}

// GetResults retrieves vote counts for a poll.
func GetResults(client *ent.Client) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		ctx := r.Context()
		// 1) Parse poll ID
		pollID, err := strconv.Atoi(ps.ByName("id"))
		if err != nil {
			http.Error(w, "invalid poll id", http.StatusBadRequest)
			return
		}
		// 2) Ensure poll exists
		exists, err := client.Poll.
			Query().
			Where(poll.IDEQ(pollID)).
			Exist(ctx)
		if err != nil {
			log.Printf("error checking poll existence: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if !exists {
			http.Error(w, "poll not found", http.StatusNotFound)
			return
		}
		// 3) Load options + their votes
		opts, err := client.PollOption.
			Query().
			Where(polloption.PollIDEQ(pollID)).
			WithVotes().
			All(ctx)
		if err != nil {
			log.Printf("error querying options: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		// 4) Build response
		type result struct {
			OptionID int    `json:"option_id"`
			Text     string `json:"text"`
			Votes    int    `json:"votes"`
		}
		results := make([]result, len(opts))
		for i, o := range opts {
			results[i] = result{
				OptionID: o.ID,
				Text:     o.Text,
				Votes:    len(o.Edges.Votes),
			}
		}
		resp := struct {
			PollID  int      `json:"poll_id"`
			Results []result `json:"results"`
		}{
			PollID:  pollID,
			Results: results,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// GetVoters retrieves users who voted for a specific option.
func GetVoters(client *ent.Client) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		ctx := r.Context()
		// 1) Parse option ID
		optID, err := strconv.Atoi(ps.ByName("optionId"))
		if err != nil {
			http.Error(w, "invalid option id", http.StatusBadRequest)
			return
		}
		// 2) Ensure option exists
		exists, err := client.PollOption.
			Query().
			Where(polloption.IDEQ(optID)).
			Exist(ctx)
		if err != nil {
			log.Printf("error checking option existence: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if !exists {
			http.Error(w, "option not found", http.StatusNotFound)
			return
		}
		// 3) Load all votes for that option, with user edges
		votes, err := client.Vote.
			Query().
			Where(vote.OptionIDEQ(optID)).
			WithUser().
			All(ctx)
		if err != nil {
			log.Printf("error querying votes: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		// 4) Build response
		type voter struct {
			UserID   int    `json:"user_id"`
			Username string `json:"username"`
		}
		voters := make([]voter, len(votes))
		for i, v := range votes {
			u := v.Edges.User
			voters[i] = voter{
				UserID:   u.ID,
				Username: u.Username,
			}
		}
		resp := struct {
			OptionID int     `json:"option_id"`
			Voters   []voter `json:"voters"`
		}{
			OptionID: optID,
			Voters:   voters,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// ListUsers retrieves all registered users
func ListUsers(client *ent.Client) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		ctx := r.Context()

		users, err := client.User.
			Query().
			All(ctx)
		if err != nil {
			http.Error(w, "failed querying users", http.StatusInternalServerError)
			log.Printf("ERROR querying users: %v\n", err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(users); err != nil {
			http.Error(w, "failed encoding response", http.StatusInternalServerError)
			return
		}
	}
}

// Logout handles user logout by clearing the "user_id" cookie.
func Logout(client *ent.Client) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		// Overwrite the cookie with an expired one to remove it from the browser
		cookie := &http.Cookie{
			Name:     "user_id",
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Expires:  time.Unix(0, 0),
			MaxAge:   -1,
		}
		http.SetCookie(w, cookie)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message":"logout successful"}`))
	}
}

// UpdatePoll allows the creator to replace a poll’s options.
// This will also delete all existing votes on that poll.
func UpdatePoll(client *ent.Client) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		ctx := r.Context()

		// 1) Authenticate via cookie
		c, err := r.Cookie("user_id")
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		userID, err := strconv.Atoi(c.Value)
		if err != nil {
			http.Error(w, "invalid user_id cookie", http.StatusUnauthorized)
			return
		}

		// 2) Parse poll ID
		pollID, err := strconv.Atoi(ps.ByName("id"))
		if err != nil {
			http.Error(w, "invalid poll id", http.StatusBadRequest)
			return
		}

		// 3) Verify ownership
		p, err := client.Poll.
			Query().
			Where(poll.IDEQ(pollID)).
			Only(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				http.Error(w, "poll not found", http.StatusNotFound)
			} else {
				log.Printf("query poll error: %v", err)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
			return
		}
		if p.CreatorID != userID {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		// 4) Decode new options
		var req struct {
			Options []string `json:"options"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON payload", http.StatusBadRequest)
			return
		}
		if len(req.Options) < 2 {
			http.Error(w, "at least two options are required", http.StatusBadRequest)
			return
		}

		// 5) Begin transaction
		tx, err := client.Tx(ctx)
		if err != nil {
			log.Printf("failed to start tx: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		rollback := func() {
			if rb := tx.Rollback(); rb != nil {
				log.Printf("tx rollback error: %v", rb)
			}
		}

		// 6) Delete all votes for this poll
		if _, err := tx.Vote.
			Delete().
			Where(vote.PollIDEQ(pollID)).
			Exec(ctx); err != nil {
			rollback()
			log.Printf("failed deleting votes: %v", err)
			http.Error(w, "could not clear votes", http.StatusInternalServerError)
			return
		}

		// 7) Delete existing options
		if _, err := tx.PollOption.
			Delete().
			Where(polloption.PollIDEQ(pollID)).
			Exec(ctx); err != nil {
			rollback()
			log.Printf("failed deleting old options: %v", err)
			http.Error(w, "could not update options", http.StatusInternalServerError)
			return
		}

		// 8) Create new options
		createdOpts := make([]*ent.PollOption, 0, len(req.Options))
		for _, text := range req.Options {
			o, err := tx.PollOption.
				Create().
				SetText(text).
				SetPoll(p).
				Save(ctx)
			if err != nil {
				rollback()
				log.Printf("failed creating option %q: %v", text, err)
				http.Error(w, "could not update options", http.StatusInternalServerError)
				return
			}
			createdOpts = append(createdOpts, o)
		}

		// 9) Commit transaction
		if err := tx.Commit(); err != nil {
			rollback()
			log.Printf("failed committing tx: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		// 10) Build response
		type optionResp struct {
			ID   int    `json:"id"`
			Text string `json:"text"`
		}
		respOpts := make([]optionResp, len(createdOpts))
		for i, o := range createdOpts {
			respOpts[i] = optionResp{ID: o.ID, Text: o.Text}
		}
		resp := struct {
			ID        int          `json:"id"`
			Title     string       `json:"title"`
			CreatorID int          `json:"creator_id"`
			Options   []optionResp `json:"options"`
		}{
			ID:        p.ID,
			Title:     p.Title,
			CreatorID: p.CreatorID,
			Options:   respOpts,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Printf("failed encoding response: %v", err)
		}
	}
}

// DeletePoll allows the creator to delete their poll (and its options & votes).
func DeletePoll(client *ent.Client) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		ctx := r.Context()

		// 1) Authenticate via cookie
		c, err := r.Cookie("user_id")
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		userID, err := strconv.Atoi(c.Value)
		if err != nil {
			http.Error(w, "invalid user_id cookie", http.StatusUnauthorized)
			return
		}

		// 2) Parse poll ID from path
		pollID, err := strconv.Atoi(ps.ByName("id"))
		if err != nil {
			http.Error(w, "invalid poll id", http.StatusBadRequest)
			return
		}

		// 3) Verify ownership
		p, err := client.Poll.
			Query().
			Where(poll.IDEQ(pollID)).
			Only(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				http.Error(w, "poll not found", http.StatusNotFound)
			} else {
				log.Printf("query poll error: %v", err)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
			return
		}
		if p.CreatorID != userID {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		// 4) Begin transaction
		tx, err := client.Tx(ctx)
		if err != nil {
			log.Printf("failed to start tx: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		rollback := func() {
			if rbErr := tx.Rollback(); rbErr != nil {
				log.Printf("tx rollback error: %v", rbErr)
			}
		}

		// 5) Delete all votes for this poll
		if _, err := tx.Vote.
			Delete().
			Where(vote.PollIDEQ(pollID)).
			Exec(ctx); err != nil {
			rollback()
			log.Printf("failed deleting votes: %v", err)
			http.Error(w, "could not delete poll", http.StatusInternalServerError)
			return
		}

		// 6) Delete all options for this poll
		if _, err := tx.PollOption.
			Delete().
			Where(polloption.PollIDEQ(pollID)).
			Exec(ctx); err != nil {
			rollback()
			log.Printf("failed deleting options: %v", err)
			http.Error(w, "could not delete poll", http.StatusInternalServerError)
			return
		}

		// 7) Delete the poll itself
		if err := tx.Poll.
			DeleteOneID(pollID).
			Exec(ctx); err != nil {
			rollback()
			log.Printf("failed deleting poll: %v", err)
			http.Error(w, "could not delete poll", http.StatusInternalServerError)
			return
		}

		// 8) Commit transaction
		if err := tx.Commit(); err != nil {
			rollback()
			log.Printf("failed committing tx: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		// 9) Return no content
		w.WriteHeader(http.StatusNoContent)
	}
}
