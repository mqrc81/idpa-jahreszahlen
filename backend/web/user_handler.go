package web

import (
	"html/template"
	"net/http"

	"github.com/alexedwards/scs/v2"
	"golang.org/x/crypto/bcrypt"

	"github.com/mqrc81/IDPA-Jahreszahlen/backend"
)

type UserHandler struct {
	store backend.Store
	sessions *scs.SessionManager
}

/*
 * Register is a GET method with form to register new user
 */
func (h *UserHandler) Register() http.HandlerFunc {
	// Parse HTML-template
	tmpl := template.Must(template.New("").Parse(usersRegisterHTML))

	return func(w http.ResponseWriter, r *http.Request) {
		// Execute HTML-template
		if err := tmpl.Execute(w, nil); err != nil {
		    http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

/*
 * RegisterSubmit is a POST method that stores user created
 */
func (h *UserHandler) RegisterSubmit() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Hash password
		password, err := bcrypt.GenerateFromPassword([]byte(r.FormValue("password")), bcrypt.DefaultCost)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Execute SQL statement
		if err := h.store.CreateUser(&backend.User{
			UserID:   0,
			Username: r.FormValue("username"),
			Password: string(password),
			Admin:    false,
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/", http.StatusFound)
	}
}
