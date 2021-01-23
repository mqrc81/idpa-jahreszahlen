// The web handler evolving around topics, with HTTP-handler functions
// consisting of "GET"- and "POST"-methods. It utilizes session management and
// database access.

package web

import (
	"html/template"
	"net/http"
	"strconv"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi"
	"github.com/gorilla/csrf"

	x "github.com/mqrc81/IDPA-Jahreszahlen/backend"
)

// TopicHandler is the object for handlers to access sessions and database.
type TopicHandler struct {
	store    x.Store
	sessions *scs.SessionManager
}

// List is a GET-method that is accessible to anyone.
//
// It lists all topics. Users can only view them or show a specific topic,
// while admins have the ability to create a new topic, as well as to edit and
// delete an existing one.
func (h *TopicHandler) List() http.HandlerFunc {

	// Data to pass to HTML-templates
	type data struct {
		SessionData
		CSRF template.HTML

		Topics []x.Topic
	}

	return func(res http.ResponseWriter, req *http.Request) {

		// Execute SQL statement to get topics
		topics, err := h.store.GetTopics()
		if err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}

		// Execute HTML-templates with data
		if err = Templates["topics_list"].Execute(res, data{
			SessionData: GetSessionData(h.sessions, req.Context()),
			CSRF:        csrf.TemplateField(req),
			Topics:      topics,
		}); err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

// Create is a GET-method that is accessible to any admin.
//
// It displays a form, in which values for a new topic can be entered.
func (h *TopicHandler) Create() http.HandlerFunc {

	// Data to pass to HTML-templates
	type data struct {
		SessionData
		CSRF template.HTML
	}

	return func(res http.ResponseWriter, req *http.Request) {

		// Check if an admin is logged in
		user := req.Context().Value("user")
		if user == nil || !user.(x.User).Admin {
			// If no user is logged in or logged in user isn't an admin,
			// then redirect back with flash message
			h.sessions.Put(req.Context(), "flash_error", "Unzureichende Berechtigung. "+
				"Sie müssen als Admin eingeloggt sein, um ein neues Thema zu erstellen.")
			http.Redirect(res, req, req.Referer(), http.StatusFound)
			return
		}

		// Execute HTML-templates with data
		if err := Templates["topics_create"].Execute(res, data{
			SessionData: GetSessionData(h.sessions, req.Context()),
			CSRF:        csrf.TemplateField(req),
		}); err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

// CreateStore is a POST-method that is accessible to.
//
// It validates the form from Create and redirects to Create in case of an
// invalid input with corresponding error message. In case of valid form, it
// stores the new topic in the database and redirects to List.
func (h *TopicHandler) CreateStore() http.HandlerFunc {

	return func(res http.ResponseWriter, req *http.Request) {

		// Retrieve values from form
		startYear, _ := strconv.Atoi(req.FormValue("start_year"))
		endYear, _ := strconv.Atoi(req.FormValue("end_year"))
		form := TopicForm{
			Name:        req.FormValue("name"),
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

		// Execute SQL statement to create a topic
		if err := h.store.CreateTopic(&x.Topic{
			Name:        form.Name,
			StartYear:   form.StartYear,
			EndYear:     form.EndYear,
			Description: form.Description,
		}); err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}

		// Adds flash message
		h.sessions.Put(req.Context(), "flash_success", "Thema wurde erfolgreich erstellt.")

		// Redirects to list of topics
		http.Redirect(res, req, "/topics", http.StatusFound)
	}
}

// Delete is a POST-method that is accessible to any admin.
//
// It deletes a certain topic and redirects to List.
func (h *TopicHandler) Delete() http.HandlerFunc {

	return func(res http.ResponseWriter, req *http.Request) {

		// Retrieve TopicID from URL parameters
		topicID, _ := strconv.Atoi(chi.URLParam(req, "topicID"))

		// Execute SQL statement to delete a topic
		if err := h.store.DeleteTopic(topicID); err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}
		// Add flash message
		h.sessions.Put(req.Context(), "flash_success", "Thema wurde erfolgreich gelöscht.")

		// Redirect to list of topics
		http.Redirect(res, req, "/topics", http.StatusFound)
	}
}

// EditPrepare is a POST-method that is accessible to any admin.
//
// It creates a form from the topic's values, so that values are already filled out
// when editing the topic.
func (h *TopicHandler) EditPrepare() http.HandlerFunc {

	return func(res http.ResponseWriter, req *http.Request) {

		// Retrieve topic ID from URL parameters
		topicIDstr := chi.URLParam(req, "topicID")
		topicID, _ := strconv.Atoi(topicIDstr)

		// Execute SQL statement to get topic
		topic, err := h.store.GetTopic(topicID)
		if err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}

		// Create form to add to session, so that values are already filled out
		// when editing a topic
		form := TopicForm{
			Name:        topic.Name,
			StartYear:   topic.StartYear,
			EndYear:     topic.EndYear,
			Description: topic.Description,
			Errors:      FormErrors{},
		}

		// Add form to session
		h.sessions.Put(req.Context(), "form", form)

		// Redirect to edit-page of topic
		http.Redirect(res, req, "/topics/"+topicIDstr+"/edit", http.StatusFound)
	}
}

// Edit is a GET-method that is accessible to any admin.
//
// It displays a form in which values for modifying the current topic can be
// entered.
func (h *TopicHandler) Edit() http.HandlerFunc {

	// Data to pass to HTML-templates
	type data struct {
		SessionData
		CSRF template.HTML

		Topic  x.Topic
		Events []x.Event
	}

	return func(res http.ResponseWriter, req *http.Request) {

		// Check if an admin is logged in
		user := req.Context().Value("user")
		if user == nil || !user.(x.User).Admin {
			// If no user is logged in or logged in user isn't an admin,
			// then redirect back with flash message
			h.sessions.Put(req.Context(), "flash_error",
				"Unzureichende Berechtigung. Sie müssen als Admin eingeloggt sein, um ein Thema zu bearbeiten.")
			http.Redirect(res, req, req.Referer(), http.StatusFound)
			return
		}

		// Retrieve topic ID from URL parameters
		topicID, err := strconv.Atoi(chi.URLParam(req, "topicID"))
		if err != nil {
			http.Error(res, err.Error(), http.StatusNotFound)
			return
		}

		// Execute SQL statement to get a topic
		topic, err := h.store.GetTopic(topicID)
		if err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}

		// Execute HTML-templates with data
		if err = Templates["topics_edit"].Execute(res, data{
			SessionData: GetSessionData(h.sessions, req.Context()),
			CSRF:        csrf.TemplateField(req),
			Topic:       topic,
		}); err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

// EditStore is a POST-method that is accessible to any admin.
//
// It validates the form from Edit and redirects to Edit in case of an invalid
// input with corresponding error message. In case of valid form, it stores the
// topic in the database and redirects to Show.
func (h *TopicHandler) EditStore() http.HandlerFunc {

	return func(res http.ResponseWriter, req *http.Request) {

		// Retrieve values from form
		startYear, _ := strconv.Atoi(req.FormValue("start_year"))
		endYear, _ := strconv.Atoi(req.FormValue("end_year"))
		form := TopicForm{
			Name:        req.FormValue("name"),
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

		// Retrieve topic ID from URL
		topicIDstr := req.URL.Query().Get("topicID")
		topicID, _ := strconv.Atoi(topicIDstr)

		// Execute SQL statement to update a topic
		if err := h.store.UpdateTopic(&x.Topic{
			TopicID:     topicID,
			Name:        form.Name,
			StartYear:   form.StartYear,
			EndYear:     form.EndYear,
			Description: form.Description,
		}); err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}

		// Add flash message
		h.sessions.Put(req.Context(), "flash_success", "Thema wurde erfolgreich bearbeitet.")

		// Redirect to topic Show
		http.Redirect(res, req, "/topics/"+topicIDstr, http.StatusFound)
	}
}

// Show is a GET-method that is accessible to anyone.
//
// It displays details of the topic. Anyone can view the topic, while users
// have the ability to play the quiz and admins have the ability to edit or
// delete the topic.
func (h *TopicHandler) Show() http.HandlerFunc {

	// Data to pass to HTML-templates
	type data struct {
		SessionData
		CSRF template.HTML

		Topic x.Topic
	}

	return func(res http.ResponseWriter, req *http.Request) {

		// Retrieve TopicID from URL parameters
		topicID, err := strconv.Atoi(chi.URLParam(req, "topicID"))
		if err != nil {
			http.Error(res, err.Error(), http.StatusNotFound)
			return
		}

		// Execute SQL statement to get a topic
		topic, err := h.store.GetTopic(topicID)
		if err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}

		// Execute HTML-templates with data
		if err = Templates["topics_show"].Execute(res, data{
			SessionData: GetSessionData(h.sessions, req.Context()),
			CSRF:        csrf.TemplateField(req),
			Topic:       topic,
		}); err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}
