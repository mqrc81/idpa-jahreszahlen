package web

/*
 * topic_handler.go contains HTTP-handler functions for topics
 */

import (
	"html/template"
	"net/http"
	"strconv"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi"

	"github.com/mqrc81/IDPA-Jahreszahlen/backend"
)

/*
 * TopicHandler handles sessions, CSRF-protection and database access for topics
 */
type TopicHandler struct {
	store    backend.Store
	sessions *scs.SessionManager
}

/*
 * List is a GET method that lists all topics
 */
func (h *TopicHandler) List() http.HandlerFunc {
	// Data to pass to HTML-template
	type data struct {
		SessionData

		Topics []backend.Topic
	}

	// Parse HTML-template
	tmpl := template.Must(template.New("").Parse(topicsListHTML))

	return func(res http.ResponseWriter, req *http.Request) {
		// Execute SQL statement and return slice of topics
		tt, err := h.store.Topics()
		if err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}

		// Execute HTML-template
		if err := tmpl.Execute(res, data{
			SessionData: GetSessionData(h.sessions, req.Context()),
			Topics:      tt,
		}); err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

/*
 * Create is a GET method for a form to create a new topic
 */
func (h *TopicHandler) Create() http.HandlerFunc {
	// Data to pass to HTML-template
	type data struct {
		SessionData
	}
	// Parse HTML-template
	tmpl := template.Must(template.New("").Parse(topicsCreateHTML))

	return func(res http.ResponseWriter, req *http.Request) {
		// Execute HTML-template
		if err := tmpl.Execute(res, data{
			SessionData: GetSessionData(h.sessions, req.Context()),
		}); err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

/*
 * Store is a POST method that stores topic created
 */
func (h *TopicHandler) Store() http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		// Retrieve variables from form (Create)
		startYear, _ := strconv.Atoi(req.FormValue("start_year"))
		endYear, _ := strconv.Atoi(req.FormValue("end_year"))
		form := CreateTopicForm{
			Title:       req.FormValue("title"),
			StartYear:   startYear,
			EndYear:     endYear,
			Description: req.FormValue("description"),
		}

		// Validate form
		if !form.Validate() {
			h.sessions.Put(req.Context(), "form", form)
			http.Redirect(res, req, req.Referer(), http.StatusFound)
			return
		}

		// Execute SQL statement
		if err := h.store.CreateTopic(&backend.Topic{
			TopicID:     0,
			Title:       form.Title,
			StartYear:   form.StartYear,
			EndYear:     form.EndYear,
			Description: form.Description,
			PlayCount:   0,
		}); err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}

		// Adds flash message
		h.sessions.Put(req.Context(), "flash", "Thema wurde erfolgreich erstellt.")

		// Redirects to list of topics
		http.Redirect(res, req, "/topics", http.StatusFound)
	}
}

/*
 * Delete is a POST method that deletes a topic
 */
func (h *TopicHandler) Delete() http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		// Retrieve TopicID from URL
		topicID, _ := strconv.Atoi(chi.URLParam(req, "topicID"))

		// Execute SQL statement
		if err := h.store.DeleteTopic(topicID); err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}

		// Redirect to list of topics
		http.Redirect(res, req, "/topics", http.StatusFound)
	}
}

/*
 * Edit is a GET method with the option to edit a specific topic and its events
 */
func (h *TopicHandler) Edit() http.HandlerFunc {
	// Data to pass to HTML-template
	type data struct {
		SessionData

		Topic  backend.Topic
		Events []backend.Event
	}

	// Parse HTML-template
	tmpl := template.Must(template.New("").Parse(topicsEditHTML))

	return func(res http.ResponseWriter, req *http.Request) {
		// Retrieve topic ID from URL
		topicID, _ := strconv.Atoi(chi.URLParam(req, "topicID"))

		// Execute SQL statement and return topic
		t, err := h.store.Topic(topicID)
		if err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}

		// Execute SQL statement and return events
		ee, err := h.store.EventsByTopic(topicID, false)
		if err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}

		// Execute HTML-template
		if err := tmpl.Execute(res, data{
			SessionData: GetSessionData(h.sessions, req.Context()),
			Topic:       t,
			Events:      ee,
		}); err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

/*
 * Show is a GET method that shows a specific topic with options to play, see leaderboard, (edit if admin)
 */
func (h *TopicHandler) Show() http.HandlerFunc {
	// Data to pass to HTML-template
	type data struct {
		SessionData

		Topic backend.Topic
	}

	// Parse HTML-template
	tmpl := template.Must(template.New("").Parse(topicsShowHTML))

	return func(res http.ResponseWriter, req *http.Request) {
		// Retrieve TopicID from URL
		topicID, _ := strconv.Atoi(chi.URLParam(req, "topicID"))

		// Execute SQL statement and return topic
		t, err := h.store.Topic(topicID)
		if err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}

		// Execute HTML-template
		if err := tmpl.Execute(res, data{
			SessionData: GetSessionData(h.sessions, req.Context()),
			Topic:       t,
		}); err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}
