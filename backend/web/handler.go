package web

// handler.go
// Contains HTTP-router and all HTTP-handlers

import (
	"context"
	"html/template"
	"net/http"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"

	"github.com/mqrc81/IDPA-Jahreszahlen/backend"
)

const (
	DefaultPassword = "pw123" // Used when admin manually resets a user's password
)

var (
	FuncMap = template.FuncMap{ // A map with functions to be used in HTML-templates
		// increments number by 1
		"increment": func(num int) int {
			return num + 1
		},
	}
)

// NewHandler
// Initializes HTTP-handlers, including router and middleware
func NewHandler(store backend.Store, sessions *scs.SessionManager) *Handler {
	handler := &Handler{
		Mux:      chi.NewMux(),
		store:    store,
		sessions: sessions,
	}

	topics := TopicHandler{store: store, sessions: sessions}
	events := EventHandler{store: store, sessions: sessions}
	scores := ScoreHandler{store: store, sessions: sessions}
	quiz := QuizHandler{store: store, sessions: sessions}
	users := UserHandler{store: store, sessions: sessions}

	// Use middleware
	handler.Use(middleware.Logger)
	handler.Use(sessions.LoadAndSave)
	handler.Use(handler.withUser)

	// Home
	handler.Get("/", handler.Home())
	handler.Get("/about", handler.About())

	// Topics
	handler.Route("/topics", func(r chi.Router) {
		r.Get("/", topics.List())
		r.Get("/new", topics.Create())
		r.Post("/", topics.CreateStore())
		r.Post("/{topicID}/delete", topics.Delete())
		r.Get("/{topicID}/edit", topics.Edit())
		r.Get("/{topicID}", topics.Show())
	})

	// Events
	handler.Route("/topics/{topicID}/events", func(router chi.Router) {
		router.Get("/", events.List())
		router.Get("/new", events.Create())
		router.Post("/", events.CreateStore())
		router.Post("/{eventID}/delete", events.Delete())
		router.Get("/edit", events.Edit())
		router.Post("/edit", events.EditStore())
	})

	// Quiz
	handler.Route("/topics/{topicID}/quiz", func(router chi.Router) {
		router.Get("/1", quiz.Phase1())
		router.Post("/1", quiz.Phase1Submit())
		router.Get("1/review", quiz.Phase1Review())
		router.Get("/2", quiz.Phase2())
		// TODO router.Post("/2", quiz.Phase2Submit())
		// TODO router.Get("2/review", quiz.Phase2Review())
		router.Get("/3", quiz.Phase3())
		// TODO router.Post("/3", quiz.Phase3Submit())
		// TODO router.Get("3/review", quiz.Phase3Review())
		router.Post("/", quiz.Store())
		router.Get("/summary", quiz.Summary())
	})

	// Scores
	handler.Route("/scores", func(router chi.Router) {
		router.Get("/", scores.List())
		router.Post("/", scores.Store())
	})

	// Users
	handler.Route("/users", func(router chi.Router) {
		router.Get("/register", users.Register())
		router.Post("/register", users.RegisterSubmit())
		router.Get("/login", users.Login())
		router.Post("/login", users.LoginSubmit())
		router.Get("/logout", users.Logout())
		router.Get("/profile", users.Profile())
		router.Get("/", users.List())
		router.Get("/{userID}/edit/username", users.EditUsername())
		router.Post("/{userID}/edit/username", users.EditUsernameSubmit())
		router.Get("/{userID}/edit/password", users.EditPassword())
		router.Post("/{userID}/edit/password", users.EditPasswordSubmit())
		router.Post("/{userID}/delete", users.Delete())
		router.Post("/{userID}/promote", users.Promote())
		router.Post("/{userID}/reset/password", users.ResetPassword())
	})

	// Handler for when the user enters a non-existing URL
	handler.NotFound(handler.NotFound404())
	// handler.MethodNotAllowed(handler.NotAllowed405())

	return handler
}

// Handler
// Consists of the chi-multiplexer, a store interface and sessions
type Handler struct {
	*chi.Mux
	store    backend.Store
	sessions *scs.SessionManager
}

// Home
// A GET-method. Renders the home-page.
func (handler *Handler) Home() http.HandlerFunc {
	// Data to pass to HTML-templates
	type data struct {
		SessionData

		Topics []backend.Topic
		Scores []backend.Score
	}

	// Parse HTML-templates
	tmpl := template.Must(template.ParseFiles(
		"frontend/layout.html",
		"frontend/home.html",
	))

	return func(res http.ResponseWriter, req *http.Request) {
		// Execute SQL statement to get topics
		tt, err := handler.store.Topics()
		if err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}

		// Execute SQL statement to get scores
		ss, err := handler.store.Scores(5, 0)
		if err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}

		// Execute HTML-templates with data
		if err := tmpl.Execute(res, data{
			SessionData: GetSessionData(handler.sessions, req.Context()),
			Topics:      tt,
			Scores:      ss,
		}); err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

// About
// A GET-method. Renders the about-page.
func (handler *Handler) About() http.HandlerFunc {
	// Data to pass to HTML-templates
	type data struct {
		SessionData
	}
	// Parse HTML-templates
	tmpl := template.Must(template.ParseFiles(
		"frontend/layout.html",
		"frontend/pages/about.html",
	))

	return func(res http.ResponseWriter, req *http.Request) {
		// Execute HTML-templates with data
		if err := tmpl.Execute(res, data{
			SessionData: GetSessionData(handler.sessions, req.Context()),
		}); err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

// withUser
// A middleware that replaces the potential user ID with a user object.
func (handler *Handler) withUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		// Retrieve user ID from session
		var userID int
		userIDinf := handler.sessions.Get(req.Context(), "user_id")
		if userIDinf != nil {
			userID = userIDinf.(int)
		}

		// Execute SQL statement to get user
		user, err := handler.store.User(userID)
		if err != nil {
			// No user in session => continue to HTTP-handler
			next.ServeHTTP(res, req)
			return
		}

		// Add the user logged in to the session
		ctx := context.WithValue(req.Context(), "user", user)

		// Serve HTTP with response-writer and request
		next.ServeHTTP(res, req.WithContext(ctx))
	})
}

// NotFound404
// Gets called when a non-existing URL has been entered.
func (handler *Handler) NotFound404() http.HandlerFunc {
	// Data to pass to HTML-templates
	type data struct {
		SessionData
	}

	// Parse HTML-templates
	tmpl := template.Must(template.ParseFiles(
		"frontend/layout.html",
		"frontend/pages/http_not_found.html",
	))
	return func(res http.ResponseWriter, req *http.Request) {
		if err := tmpl.Execute(res, data{
			SessionData: GetSessionData(handler.sessions, req.Context()),
		}); err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}
