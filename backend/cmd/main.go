// The main file, which acquires a connection to the database and the
// server. It also obtains session management and CSRF-protection.

package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi"
	"github.com/joho/godotenv"

	"github.com/mqrc81/IDPA-Jahreszahlen/backend/database"
	"github.com/mqrc81/IDPA-Jahreszahlen/backend/util"
	"github.com/mqrc81/IDPA-Jahreszahlen/backend/web"
)

// main is the initial starting point of the program, which acquires a
// connection to the database and the server. It also obtains session
// management and CSRF-protection.
func main() {
	port := ":3000"
	fmt.Println("Starting application...")

	// Access global environment variables
	if err := godotenv.Load("backend/.env"); err != nil {
		log.Fatalf("error loading environment variables: %v", err)
	}

	// Get data-source-name from environment variables
	dataSourceName := os.Getenv("DB_DSN")

	// Establish database connection with the help of the data-source-name
	store, err := database.NewStore(dataSourceName)
	if err != nil {
		log.Fatalf("error initializing new database store: %v", err)
	}

	// Initialize session manager
	sessions, err := web.NewSessionManager(dataSourceName)
	if err != nil {
		log.Fatalf("error initializing new session manager: %v", err)
	}

	// Generate random 32-byte key for CSRF-protection
	csrfKey := util.GenerateBytes(32)

	// Initialize HTTP-handlers, including router and middleware
	handler := web.NewHandler(store, sessions, csrfKey)

	FileServer(handler, "/frontend/static/css", http.Dir("frontend/static/css"))

	// Listen on the TCP network address and call Serve with handler to handle
	// requests on incoming connections
	fmt.Println("Listening on port " + port + "...")
	if err = http.ListenAndServe(port, handler); err != nil {
		log.Fatalf("error listening on the tcp network: %v", err)
	}
}

// FileServer conveniently sets up a http.FileServer handler to serve
// static files, like CSS, Images or JavaScript
func FileServer(r chi.Router, path string, root http.FileSystem) {
	if strings.ContainsAny(path, "{}*") {
		panic("FileServer does not permit any URL parameters.")
	}
	fmt.Println("Serving static files")

	if path != "/" && path[len(path)-1] != '/' {
		r.Get(path, http.RedirectHandler(path+"/", http.StatusMovedPermanently).ServeHTTP)
		path += "/"
	}
	path += "*"

	r.Get(path, func(w http.ResponseWriter, r *http.Request) {
		rctx := chi.RouteContext(r.Context())
		pathPrefix := strings.TrimSuffix(rctx.RoutePattern(), "/*")
		fs := http.StripPrefix(pathPrefix, http.FileServer(root))
		fs.ServeHTTP(w, r)
	})
}
