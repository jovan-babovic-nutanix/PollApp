// router/router.go
package router

import (
	"pollAppNew/ent"

	"pollAppNew/internal/handler"

	"github.com/julienschmidt/httprouter"
)

func Setup(client *ent.Client) *httprouter.Router {
	r := httprouter.New()

	// Auth routes
	r.POST("/signup", handler.SignUp(client))
	r.POST("/login", handler.Login(client))

	// Poll routes
	r.POST("/polls", handler.CreatePoll(client))
	r.GET("/polls/:id", handler.GetPoll(client))
	r.GET("/polls", handler.ListPolls(client))

	// Voting routes
	r.POST("/polls/:id/vote", handler.Vote(client))
	r.GET("/polls/:id/results", handler.GetResults(client))
	r.GET("/polls/:id/results/:optionId/voters", handler.GetVoters(client))

	//added
	// User routes
	r.GET("/users", handler.ListUsers(client))
	// Logout route
	r.POST("/logout", handler.Logout(client))
	//Mdify poll route
	r.PUT("/polls/:id", handler.UpdatePoll(client))
	// Delete poll route
	r.DELETE("/polls/:id", handler.DeletePoll(client))

	return r
}
