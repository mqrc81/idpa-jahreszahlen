// The web handler evolving around playing a quiz, with HTTP-handler functions
// consisting of "GET"- and "POST"-methods. It utilizes session management and
// database access.

package web

import (
	"encoding/gob"
	"fmt"
	"html/template"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi"
	"github.com/gorilla/csrf"

	x "github.com/mqrc81/IDPA-Jahreszahlen/backend"
	"github.com/mqrc81/IDPA-Jahreszahlen/backend/util"
)

const (
	timeExpiry = 20 // max time to be spent in a specific phase of a quiz

	p1Questions      = 3  // amount of questions in phase 1
	p1Choices        = 3  // amount of choices per question of phase 1
	p1Points         = 3  // amount of points per correct guess of phase 1
	p1ChoicesMaxDiff = 10 // highest possible difference between the correct year and a random year of phase 1

	p2Questions     = 4 // amount of questions in phase 2
	p2Points        = 8 // amount of points per correct guess of phase 2
	p2PartialPoints = 3 // amount of partial points possible in phase 2, when guess was incorrect, but close

	p3Points = 5
)

var (
	// Parsed HTML-templates to be executed in their respective HTTP-handler
	// functions when needed
	quizPhase1Template, quizPhase1ReviewTemplate, quizPhase2Template, quizPhase2ReviewTemplate, quizPhase3Template,
	quizPhase3ReviewTemplate, quizSummaryTemplate *template.Template
)

// init gets initialized with the package.
//
// It registers certain types to the session, because by default the session
// can only contain basic data types (int, bool, string, etc.).
//
// All HTML-templates get parsed once to be executed when needed. This is way
// more efficient than parsing the HTML-templates with every request.
func init() {
	gob.Register(QuizData{})
	gob.Register(x.Topic{})
	gob.Register([]x.Event{})
	gob.Register(x.Event{})
	gob.Register([]phase1Question{})
	gob.Register(phase1Question{})
	gob.Register([]int{})
	gob.Register([]phase2Question{})
	gob.Register(phase2Question{})
	gob.Register([]phase3Question{})
	gob.Register(phase3Question{})

	if _testing { // skip initialization of templates when running tests
		return
	}

	quizPhase1Template = template.Must(template.ParseFiles(layout, css, path+"quiz_phase1.html"))
	quizPhase1ReviewTemplate = template.Must(template.ParseFiles(layout, css, path+"quiz_phase1_review.html"))
	quizPhase2Template = template.Must(template.ParseFiles(layout, css, path+"quiz_phase2.html"))
	quizPhase2ReviewTemplate = template.Must(template.ParseFiles(layout, css, path+"quiz_phase2_review.html"))
	quizPhase3Template = template.Must(template.ParseFiles(layout, css, path+"quiz_phase3.html"))
	quizPhase3ReviewTemplate = template.Must(template.ParseFiles(layout, css, path+"quiz_phase3_review.html"))
	quizSummaryTemplate = template.Must(template.ParseFiles(layout, css, path+"quiz_summary.html"))
}

// QuizHandler is the object for handlers to access sessions and database.
type QuizHandler struct {
	store    x.Store
	sessions *scs.SessionManager
}

// QuizData contains the topic with the array of events and the points to keep
// track of, as well as the equivalent of a token (consisting of the topic ID,
// time expiry and current phase) in order to validate the correct playing
// order of a quiz.
type QuizData struct {
	Topic          x.Topic // contains topic ID for validation and events for playing the quiz
	Points         int
	CorrectGuesses int

	Questions interface{} // questions for each of the 3 phases

	Phase     int       // ensures the correct playing order, so that a user can't skip any phase
	Reviewed  bool      // ensures a user can't skip a reviewing phase
	TimeStamp time.Time // ensures a user can't return to a quiz after n minutes
}

// Phase1 is a GET-method that is accessible to any user.
//
// It consists of a form with 3 multiple-choice questions, where the user has
// to guess the year of a given event.
func (h *QuizHandler) Phase1() http.HandlerFunc {

	// Data to pass to HTML-templates
	type data struct {
		SessionData
		CSRF template.HTML

		TopicID   int
		TopicName string
		Questions []phase1Question
	}

	return func(res http.ResponseWriter, req *http.Request) {

		// Retrieve topic ID from URL parameters
		topicIDstr := chi.URLParam(req, "topicID")
		topicID, err := strconv.Atoi(topicIDstr)
		if err != nil {
			http.Error(res, err.Error(), http.StatusNotFound)
			return
		}

		// Check if a user is logged in
		user := req.Context().Value("user")
		if user == nil {
			// If no user is logged in, then redirect back with flash message
			h.sessions.Put(req.Context(), "flash_error", "Unzureichende Berechtigung. "+
				"Sie müssen als Benutzer eingeloggt sein, um ein Quiz zu spielen.")
			http.Redirect(res, req, "/topics/"+topicIDstr, http.StatusFound)
			return
		}

		// Execute SQL statement to get topic
		topic, err := h.store.GetTopic(topicID)
		if err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}

		// Shuffle array of events
		rand.Seed(time.Now().UnixNano()) // generate new seed to base RNG off of
		rand.Shuffle(len(topic.Events), func(n1, n2 int) {
			topic.Events[n1], topic.Events[n2] = topic.Events[n2], topic.Events[n1]
		})

		// For each of the first 3 events in the array, generate 2 other random
		// years for the user to guess from and to use in HTML-templates
		questions := createPhase1Questions(topic.Events)

		// Create quiz data and pass it to session
		h.sessions.Put(req.Context(), "quiz", QuizData{
			Topic:     topic,
			Questions: questions,
		})

		// Execute HTML-templates with data
		if err = quizPhase1Template.Execute(res, data{
			SessionData: GetSessionData(h.sessions, req.Context()),
			CSRF:        csrf.TemplateField(req),
			TopicID:     topicID,
			TopicName:   topic.Name,
			Questions:   questions,
		}); err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

// Phase1Submit is a POST-method that is accessible to any user after Phase1.
//
// It calculates the points and redirects to Phase1Review.
func (h *QuizHandler) Phase1Submit() http.HandlerFunc {

	return func(res http.ResponseWriter, req *http.Request) {

		// Retrieve topic ID from URL parameters
		topicIDstr := chi.URLParam(req, "topicID")

		// Retrieve quiz data from session
		quiz := h.sessions.Get(req.Context(), "quiz").(QuizData)
		questions := quiz.Questions.([]phase1Question)

		// Update quiz data
		quiz.Phase = 1
		quiz.Reviewed = false
		quiz.TimeStamp = time.Now()

		// Loop through the 3 input forms of radio-buttons of phase 1
		for num := 0; num < p1Questions; num++ {

			// Retrieve user's guess from form
			guess, _ := strconv.Atoi(req.FormValue("q" + strconv.Itoa(num)))
			questions[num].UserGuess = guess

			// Check if the user's guess is correct, by comparing it to the
			// corresponding event in the array of events of the topic
			if guess == quiz.Topic.Events[num].Year { // if guess is correct...
				quiz.CorrectGuesses++
				quiz.Points += p1Points // ...user gets 3 points
				questions[num].CorrectGuess = true
			}
		}
		quiz.Questions = questions

		// Add data to session again
		h.sessions.Put(req.Context(), "quiz", quiz)

		// Redirect to review of phase 1
		http.Redirect(res, req, "/topics/"+topicIDstr+"/quiz/1/review", http.StatusFound)
	}
}

// Phase1Review is a GET-method that is accessible to any user after Phase1.
//
// It displays a correction of the questions.
func (h *QuizHandler) Phase1Review() http.HandlerFunc {

	// Data to pass to HTML-templates
	type data struct {
		SessionData
		CSRF template.HTML

		TopicID   int
		TopicName string
		Questions []phase1Question
	}

	return func(res http.ResponseWriter, req *http.Request) {

		// Retrieve topic ID from URL parameters
		topicIDstr := chi.URLParam(req, "topicID")
		topicID, err := strconv.Atoi(topicIDstr)
		if err != nil {
			http.Error(res, err.Error(), http.StatusNotFound)
			return
		}

		// Check if a user is logged in
		user := req.Context().Value("user")
		if user == nil {
			// If no user is logged in, then redirect back with flash message
			h.sessions.Put(req.Context(), "flash_error", "Unzureichende Berechtigung. "+
				"Sie müssen als Benutzer eingeloggt sein, um ein Quiz zu spielen.")
			http.Redirect(res, req, "/topics/"+topicIDstr, http.StatusFound)
			return
		}

		// Retrieve quiz data from session
		// 'ok' is false if quiz from session isn't convertible to quizData
		// struct (so if quiz doesn't exist in session)
		quiz, ok := h.sessions.Get(req.Context(), "quiz").(QuizData)

		// Validate the token of the quiz-data
		msg := quiz.validate(ok, 1, false, topicID)

		// If 'msg' isn't empty, an error occurred
		if msg != "" {
			h.sessions.Put(req.Context(), "flash_error", fmt.Sprintf(msg, 1))
			http.Redirect(res, req, "/topics/"+topicIDstr, http.StatusFound)
			return
		}

		// Pass quiz data to session for later phases
		h.sessions.Put(req.Context(), "quiz", quiz)

		// Execute HTML-templates with data
		if err = quizPhase1ReviewTemplate.Execute(res, data{
			SessionData: GetSessionData(h.sessions, req.Context()),
			CSRF:        csrf.TemplateField(req),
			TopicID:     topicID,
			TopicName:   quiz.Topic.Name,
			Questions:   quiz.Questions.([]phase1Question),
		}); err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

// Phase2Prepare is a POST-method that is accessible to any user after
// Phase1Review.
//
// It prepares the questions to be used in Phase2 and updates the quiz data for
// future validation. This method allows user to refresh Phase2, without quiz
// data becoming invalid or questions changing.
func (h *QuizHandler) Phase2Prepare() http.HandlerFunc {

	return func(res http.ResponseWriter, req *http.Request) {

		// Retrieve topic ID from session
		topicID := chi.URLParam(req, "topicID")

		// Retrieve quiz data from session
		quiz := h.sessions.Get(req.Context(), "quiz").(QuizData)

		// Update quiz data and add questions for phase 2
		quiz.Phase = 1
		quiz.Reviewed = true
		quiz.TimeStamp = time.Now()
		// For each of the 4 events in the array, create a question to use in
		// HTML-templates
		quiz.Questions = createPhase2Questions(quiz.Topic.Events)

		// Pass quiz data to session
		h.sessions.Put(req.Context(), "quiz", quiz)

		// Redirect to phase 2 of quiz
		http.Redirect(res, req, "topics/"+topicID+"/quiz/2", http.StatusFound)
	}
}

// Phase2 is a GET-method that is accessible to any user after Phase1Review.
//
// It consists of a form with 4 questions, where the user has to guess the
// exact year of a given event.
func (h *QuizHandler) Phase2() http.HandlerFunc {

	// Data to pass to HTML-templates
	type data struct {
		SessionData
		CSRF template.HTML

		TopicID   int
		TopicName string
		Questions []phase2Question
	}

	return func(res http.ResponseWriter, req *http.Request) {

		// Retrieve topic ID from URL parameters
		topicIDstr := chi.URLParam(req, "topicID")
		topicID, err := strconv.Atoi(topicIDstr)
		if err != nil {
			http.Error(res, err.Error(), http.StatusNotFound)
			return
		}

		// Check if a user is logged in
		user := req.Context().Value("user")
		if user == nil {
			// If no user is logged in, then redirect back with flash message
			h.sessions.Put(req.Context(), "flash_error", "Unzureichende Berechtigung. "+
				"Sie müssen als Benutzer eingeloggt sein, um ein Quiz zu spielen.")
			http.Redirect(res, req, "topics/"+topicIDstr, http.StatusFound)
			return
		}

		// Retrieve quiz data from session
		// 'ok' is false if quiz from session isn't convertible to quizData
		// struct (so if quiz doesn't exist in session)
		quiz, ok := h.sessions.Get(req.Context(), "quiz").(QuizData)

		// Validate the token of the quiz-data
		msg := quiz.validate(ok, 1, true, topicID)

		// If 'msg' isn't empty, an error occurred
		if msg != "" {
			h.sessions.Put(req.Context(), "flash_error", fmt.Sprintf(msg, 2))
			http.Redirect(res, req, "/topics/"+topicIDstr, http.StatusFound)
			return
		}

		// Pass quiz data to session
		h.sessions.Put(req.Context(), "quiz", quiz)

		// Execute HTML-templates with data
		if err = quizPhase2Template.Execute(res, data{
			SessionData: GetSessionData(h.sessions, req.Context()),
			CSRF:        csrf.TemplateField(req),
			TopicID:     topicID,
			TopicName:   quiz.Topic.Name,
			Questions:   quiz.Questions.([]phase2Question),
		}); err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

// Phase2Submit is a POST-method that is accessible to any user after Phase2.
//
// It calculates the points and redirects to Phase2Review.
func (h *QuizHandler) Phase2Submit() http.HandlerFunc {

	return func(res http.ResponseWriter, req *http.Request) {

		// Retrieve topic ID from URL parameters
		topicIDstr := chi.URLParam(req, "topicID")

		// Retrieve quiz data from session
		quiz := h.sessions.Get(req.Context(), "quiz").(QuizData)
		questions := quiz.Questions.([]phase2Question)

		// Update quiz data
		quiz.Phase = 2
		quiz.Reviewed = false
		quiz.TimeStamp = time.Now()

		// Loop through the 4 input fields of phase 2
		for num := 0; num < p2Questions; num++ {

			// Retrieve user's guess from form
			guess, _ := strconv.Atoi(req.FormValue("q" + strconv.Itoa(num)))

			// Check if the user's guess is correct, by comparing it to the
			// corresponding event in the array of events of the topic
			correctYear := quiz.Topic.Events[num+p1Questions].Year
			if guess == correctYear { // if guess is correct...
				quiz.CorrectGuesses++
				quiz.Points += p2Points            // ...user gets 8 points
				questions[num].CorrectGuess = true // ...change value for that question
			} else {
				// Get absolute value of difference between user's guess and
				// correct year
				difference := util.Abs(correctYear - guess)

				// Check if the user's guess is close and potentially add
				// partial points (the closer the guess, the more points)
				if difference < p2PartialPoints { // if guess is close...
					quiz.Points += p2PartialPoints - difference // ...user gets partial points
				}
			}
		}
		quiz.Questions = questions

		// Pass quiz data to session again
		h.sessions.Put(req.Context(), "quiz", quiz)

		// Redirect to review of phase 2
		http.Redirect(res, req, "/topics/"+topicIDstr+"/quiz/2/review", http.StatusFound)
	}
}

// Phase2Review is a GET-method that is accessible to any user after Phase2.
//
// It displays a correction of the questions.
func (h *QuizHandler) Phase2Review() http.HandlerFunc {

	// Data to pass to HTML-templates
	type data struct {
		SessionData
		CSRF template.HTML

		TopicID   int
		TopicName string
		Questions []phase2Question
	}

	return func(res http.ResponseWriter, req *http.Request) {

		// Retrieve topic ID from URL parameters
		topicIDstr := chi.URLParam(req, "topicID")
		topicID, err := strconv.Atoi(topicIDstr)
		if err != nil {
			http.Error(res, err.Error(), http.StatusNotFound)
			return
		}

		// Check if a user is logged in
		user := req.Context().Value("user")
		if user == nil {
			// If no user is logged in, then redirect back with flash message
			h.sessions.Put(req.Context(), "flash_error", "Unzureichende Berechtigung. "+
				"Sie müssen als Benutzer eingeloggt sein, um ein Quiz zu spielen.")
			http.Redirect(res, req, "/topics/"+topicIDstr, http.StatusFound)
			return
		}

		// Retrieve quiz data from session
		// 'ok' is false if quiz from session isn't convertible to quizData
		// struct (so if quiz doesn't exist in session)
		quiz, ok := h.sessions.Get(req.Context(), "quiz").(QuizData)

		// Validate the token of the quiz-data
		msg := quiz.validate(ok, 2, false, topicID)

		// If 'msg' isn't empty, an error occurred
		if msg != "" {
			h.sessions.Put(req.Context(), "flash_error", fmt.Sprintf(msg, 2))
			http.Redirect(res, req, "/topics", http.StatusFound)
			return
		}

		// Pass quiz data to session for later phases
		h.sessions.Put(req.Context(), "quiz", quiz)

		// Execute HTML-templates with data
		if err = quizPhase2ReviewTemplate.Execute(res, data{
			SessionData: GetSessionData(h.sessions, req.Context()),
			CSRF:        csrf.TemplateField(req),
			TopicID:     topicID,
			TopicName:   quiz.Topic.Name,
			Questions:   quiz.Questions.([]phase2Question),
		}); err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

// Phase3Prepare is a POST-method that is accessible to any user after
// Phase2Review.
//
// It prepares the questions to be used in Phase3 and updates the quiz data for
// future validation. This method allows user to refresh Phase3, without quiz
// data becoming invalid or questions changing.
func (h *QuizHandler) Phase3Prepare() http.HandlerFunc {

	return func(res http.ResponseWriter, req *http.Request) {

		// Retrieve topic ID from session
		topicID := chi.URLParam(req, "topicID")

		// Retrieve quiz data from session
		quiz := h.sessions.Get(req.Context(), "quiz").(QuizData)

		// Update quiz data and add questions for phase 3
		quiz.Phase = 2
		quiz.Reviewed = true
		quiz.TimeStamp = time.Now()
		// For each of the events in the array, create a question to use in
		// HTML-templates
		// This includes marking the order of the events for future calculation
		// of the user's points and shuffling them
		quiz.Questions = createPhase3Questions(quiz.Topic.Events)

		// Pass quiz data to session
		h.sessions.Put(req.Context(), "quiz", quiz)

		// Redirect to phase 2 of quiz
		http.Redirect(res, req, "topics/"+topicID+"/quiz/3", http.StatusFound)
	}
}

// Phase3 is a GET-method that is accessible to any user after Phase2Review.
//
// It consists of a form with all events of the topic, where the user has to
// put the events in chronological order.
func (h *QuizHandler) Phase3() http.HandlerFunc {
	// Data to pass to HTML-templates
	type data struct {
		SessionData
		CSRF template.HTML

		TopicID   int
		TopicName string
		Questions []phase3Question
	}

	return func(res http.ResponseWriter, req *http.Request) {

		// Retrieve topic ID from URL parameters
		topicIDstr := chi.URLParam(req, "topicID")
		topicID, err := strconv.Atoi(topicIDstr)
		if err != nil {
			http.Error(res, err.Error(), http.StatusNotFound)
			return
		}

		// Check if a user is logged in
		user := req.Context().Value("user")
		if user == nil {
			// If no user is logged in, then redirect back with flash message
			h.sessions.Put(req.Context(), "flash_error", "Unzureichende Berechtigung. "+
				"Sie müssen als Benutzer eingeloggt sein, um ein Quiz zu spielen.")
			http.Redirect(res, req, "/topics/"+topicIDstr, http.StatusFound)
			return
		}

		// Retrieve quiz data from session
		// 'ok' is false if quiz from session isn't convertible to quizData
		// struct (so if quiz doesn't exist in session)
		quiz, ok := h.sessions.Get(req.Context(), "quiz").(QuizData)

		// Validate the token of the quiz-data
		msg := quiz.validate(ok, 2, true, topicID)

		// If 'msg' isn't empty, an error occurred
		if msg != "" {
			h.sessions.Put(req.Context(), "flash_error", fmt.Sprintf(msg, 3))
			http.Redirect(res, req, "/topics/"+topicIDstr, http.StatusFound)
			return
		}

		// Pass quiz data to session
		h.sessions.Put(req.Context(), "quiz", quiz)

		// Execute HTML-templates with data
		if err = quizPhase3Template.Execute(res, data{
			SessionData: GetSessionData(h.sessions, req.Context()),
			CSRF:        csrf.TemplateField(req),
			TopicID:     topicID,
			TopicName:   quiz.Topic.Name,
			Questions:   quiz.Questions.([]phase3Question),
		}); err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

// Phase3Submit is a POST-method that is accessible to any user after Phase3.
//
// It calculates the points and redirects to Phase3Review. It also creates a
// new score object which is stored in the database.
func (h *QuizHandler) Phase3Submit() http.HandlerFunc {

	return func(res http.ResponseWriter, req *http.Request) {

		// Retrieve topic ID from URL parameters
		topicID := chi.URLParam(req, "topicID")

		// Retrieve quiz data from session
		quiz := h.sessions.Get(req.Context(), "quiz").(QuizData)
		questions := quiz.Questions.([]phase3Question)

		// Update quiz data
		quiz.Phase = 3
		quiz.Reviewed = false
		quiz.TimeStamp = time.Now()

		// Create map of questions to check a question's order given its name
		questionsMap := make(map[string]int)
		for _, question := range questions {
			questionsMap[question.EventName] = question.Order
		}

		// Loop through user's order of events of phase 3
		for num := 0; num < len(questions); num++ {
			// Retrieve user's guess from form
			guess := req.FormValue("q" + strconv.Itoa(num))

			order := questionsMap[guess] // correct order of the event

			// Get absolute value of difference between user's guess and
			// correct order
			difference := util.Abs(order - num) // num represents the user's order

			// Check if guess was correct
			if difference == 0 {
				quiz.CorrectGuesses++
				questions[num].CorrectGuess = true
			}

			// User gets a max of 5 potential points, -1 per differ of order
			// Example: order of event: 7, user's guess of order of event: 10
			// => user gets 2 points (5-[10-7])
			quiz.Points += p3Points - difference
		}

		// Retrieve user from session
		user := req.Context().Value("user").(x.User)

		// Add score of quiz to database
		if err := h.store.CreateScore(&x.Score{
			TopicID: quiz.Topic.TopicID,
			UserID:  user.UserID,
			Points:  quiz.Points,
			Date:    time.Now(),
		}); err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}

		// Redirect to review of phase 3
		http.Redirect(res, req, "/topics/"+topicID+"/quiz/3/review", http.StatusFound)
	}
}

// Phase3Review is a GET-method that is accessible to any user after Phase3.
//
// It displays a correction of the questions.
func (h *QuizHandler) Phase3Review() http.HandlerFunc {

	// Data to pass to HTML-templates
	type data struct {
		SessionData
		CSRF template.HTML

		TopicID   int
		TopicName string
		Questions []phase3Question
	}

	return func(res http.ResponseWriter, req *http.Request) {

		// Retrieve topic ID from URL parameters
		topicIDstr := chi.URLParam(req, "topicID")
		topicID, err := strconv.Atoi(topicIDstr)
		if err != nil {
			http.Error(res, err.Error(), http.StatusNotFound)
			return
		}

		// Check if a user is logged in
		user := req.Context().Value("user")
		if user == nil {
			// If no user is logged in, then redirect back with flash message
			h.sessions.Put(req.Context(), "flash_error", "Unzureichende Berechtigung. "+
				"Sie müssen als Benutzer eingeloggt sein, um ein Quiz zu spielen.")
			http.Redirect(res, req, "/topics/"+topicIDstr, http.StatusFound)
			return
		}

		// Retrieve quiz data from session
		// 'ok' is false if quiz from session isn't convertible to quizData
		// struct (so if quiz doesn't exist in session)
		quiz, ok := h.sessions.Get(req.Context(), "quiz").(QuizData)

		// Validate the token of the quiz-data
		msg := quiz.validate(ok, 3, false, topicID)

		// If 'msg' isn't empty, an error occurred
		if msg != "" {
			h.sessions.Put(req.Context(), "flash_error", fmt.Sprintf(msg, 3))
			http.Redirect(res, req, "/topics", http.StatusFound)
			return
		}

		// Pass quiz data to session for summary
		h.sessions.Put(req.Context(), "quiz", quiz)

		// Execute HTML-templates with data
		if err = quizPhase3ReviewTemplate.Execute(res, data{
			SessionData: GetSessionData(h.sessions, req.Context()),
			CSRF:        csrf.TemplateField(req),
			Questions:   quiz.Questions.([]phase3Question),
		}); err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

// Summary is a GET-method that is accessible to any user after Phase3Review.
//
// It summarizes the quiz completed.
func (h *QuizHandler) Summary() http.HandlerFunc {
	// Data to pass to HTML-templates
	type data struct {
		SessionData

		Quiz              QuizData
		QuestionsCount    int
		PotentialPoints   int
		AverageComparison int
		OverAverage       bool
	}

	return func(res http.ResponseWriter, req *http.Request) {

		// Retrieve topic ID from session
		topicIDstr := chi.URLParam(req, "topicID")
		topicID, err := strconv.Atoi(topicIDstr)
		if err != nil {
			http.Error(res, err.Error(), http.StatusNotFound)
			return
		}

		// Retrieve quiz data from session
		// 'ok' is false if quiz from session isn't convertible to quizData
		// struct (so if quiz doesn't exist in session)
		quiz, ok := h.sessions.Get(req.Context(), "quiz").(QuizData)

		// Validate the token of the quiz-data
		msg := quiz.validate(ok, 1, false, topicID)

		// If 'msg' isn't empty, an error occurred
		if msg != "" {
			h.sessions.Put(req.Context(), "flash_error", fmt.Sprintf(msg, 3))
			http.Redirect(res, req, "/topics/"+topicIDstr, http.StatusFound)
			return
		}

		// Get average score for this topic from database
		scores, err := h.store.GetScoresByTopic(topicID)
		if err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}

		// Compare user's points to all previous points by calculating how many
		// users were worse than the current user
		// Example: 50 scores, 20 scores have lower points than user => user is
		// better than 40% of players (-> 20*100/50 = 40%)
		under := 0
		for under < len(scores) && quiz.Points > scores[under].Points {
			// FIXME
		}
		averageComparison := (under + 1) * 100 / (len(scores) + 1)

		// Execute HTML-templates with data
		if err = quizSummaryTemplate.Execute(res, data{
			SessionData:       GetSessionData(h.sessions, req.Context()),
			Quiz:              quiz,
			QuestionsCount:    p1Questions + p2Questions + len(quiz.Topic.Events),
			PotentialPoints:   p1Questions*p1Points + p2Questions*p2Points + len(quiz.Topic.Events)*p3Points,
			AverageComparison: averageComparison, // Example: "Du warst besser als 22% der Spieler bei diesem Thema."
		}); err != nil {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

// validate validates the correct playing order of a quiz by first checking for
// a valid quiz-data structure and then comparing the phase, topic and time-
// stamp of the quiz-data in the session with the URL and current time
// respectively. It an empty string if everything checks out or an error
// message to be used in the error flash message after redirecting back.
func (quiz QuizData) validate(ok bool, phase int, reviewed bool, topicID int) string {

	msg := "Ein Fehler ist aufgetreten in Phase %v des Quizzes. "

	// Check for invalid conversion from interface to quiz-data struct
	if !ok {
		// Occurs when a user manually enters a URL of a later phase without
		// properly starting a quiz at phase 1
		return msg + "Bitte starten Sie ein Quiz nur über die Themenübersicht."
	}

	// Check for invalid topic ID
	if topicID != quiz.Topic.TopicID {
		// Occurs when a user manually changes the topic ID in the URL whilst
		// in a later phase of a quiz
		// Example: "/topics/1/quiz/2/review" -> "/topics/11/quiz/2/review"
		return msg + "Womöglich haben Sie versucht, während des Quizzes das Thema zu ändern."
	}

	// Check for invalid phase
	if phase != quiz.Phase || reviewed != quiz.Reviewed {
		// Occurs when a user manually changes the phase in the URL
		// Example: "/topics/1/quiz/1" -> "/topics/1/quiz/3"
		return msg + "Womöglich haben Sie versucht, eine Phase des Quizzes zu überspringen."
	}

	// Check for invalid time stamp. Unix() displays the time passed in seconds
	// since a specific date. By adding the time stamp of the quiz data to the
	// expiry time, we can check if it was surpassed by the current time
	if time.Now().After(quiz.TimeStamp.Add(time.Minute * timeExpiry)) {
		// Occurs when a user refreshes URL or comes back to URL of a active
		// quiz after 20 minutes have passed
		// A user can still take more than the 20 minutes in a phase however
		return msg + fmt.Sprintf("Womöglich haben Sie das Quiz verlassen und dann versucht, "+
			"nach über %v Minuten zurückzukehren.", timeExpiry)
	}

	return ""
}

// phase1Question represents 1 of the 3 multiple-choice questions of phase 1.
// It contains name of event, year of event and 2 random years randomly mixed
// in with the correct year.
type phase1Question struct {
	EventName string // name of event
	EventYear int    // year of event
	Choices   []int  // choices in random order (including correct year)

	UserGuess    int    // only relevant for review of phase 1
	CorrectGuess bool   // only relevant of review of phase 1
	ID           string // only relevant for HTML input form name
}

// createPhase1Questions generates 3 phase1Question structures by generating
// 2 random years for each of the first 3 events in the array.
func createPhase1Questions(events []x.Event) []phase1Question {
	var questions []phase1Question

	// Set seed to generate random numbers from
	rand.Seed(time.Now().UnixNano())

	// Loop through events 0-2 and turn them into questions
	for index, event := range events[:p1Questions] { // events[:3] -> 0-2

		correctYear := event.Year // the event's year

		min := correctYear - p1ChoicesMaxDiff // minimum cap of random number
		max := correctYear + p1ChoicesMaxDiff // maximum cap of random number

		years := []int{correctYear}                 // array of years
		yearsMap := map[int]bool{correctYear: true} // map of years to ascertain uniqueness of each year

		// Generate unique, random numbers between max and min, to mix with the correct year
		rand.Seed(time.Now().Unix()) // set a seed to base RNG off of
		for c := 1; c < p1Choices; c++ {
			year := rand.Intn(max-min+1) + min // generate a random number between min and max

			if !yearsMap[year] {
				years = append(years, year) // add newly generated year to array of years
				yearsMap[year] = true
			} else {
				c-- // redo generating the previous year
			}
		}

		// Shuffle the years, so that the correct year isn't always in the
		// first spot
		rand.Shuffle(len(years), func(n1, n2 int) {
			years[n1], years[n2] = years[n2], years[n1]
		})

		// Add values to structure
		questions = append(questions, phase1Question{
			EventName: event.Name,
			EventYear: event.Year,
			Choices:   years,
			ID:        "q" + strconv.Itoa(index), // sample ID of first question: q0
		})
	}

	return questions
}

// phase2Question represents 1 of the 4 questions of phase 2. It contains name
// of event and year of event.
type phase2Question struct {
	EventName string // name of event
	EventYear int    // year of event

	CorrectGuess bool   // only relevant for review of phase 2
	ID           string // only relevant for HTML input form name
}

// createPhase2Questions generates 4 phase2Question structures for events 3-7
// respectively of the array of events of the topic.
func createPhase2Questions(events []x.Event) []phase2Question {
	var questions []phase2Question

	// Loop through events 3-7 and turn them into questions
	for index, event := range events[p1Questions:(p2Questions + p1Questions + 1)] { // events[3:8] -> 3-7
		questions = append(questions, phase2Question{
			EventName: event.Name,
			EventYear: event.Year,
			ID:        "q" + strconv.Itoa(index), // sample ID of first question: q0
		})
	}

	return questions
}

// phase3Question represents 1 of the events in the timeline of phase 3. It
// contains name of event and year of event.
type phase3Question struct {
	EventName string // name of event
	EventYear int    // year of event
	Order     int    // nth smallest year of all events

	CorrectGuess bool // only relevant for review of phase 2
}

// createPhase3Questions generates a phase3Question structure for all events of
// the topic.
func createPhase3Questions(events []x.Event) []phase3Question {
	var questions []phase3Question

	// Sort array of events by date, in order to add 'order' value to questions
	sort.Slice(events, func(n1, n2 int) bool {
		return events[n1].Date.Unix() < events[n2].Date.Unix()
	})

	// Loop through all events and turn them into questions
	for index, event := range events {
		questions = append(questions, phase3Question{
			EventName: event.Name,
			EventYear: event.Year,
			Order:     index, // event with earliest year will have order '0'
		})
	}

	// Shuffle array of questions
	rand.Seed(time.Now().UnixNano()) // generate new seed to base RNG off of
	rand.Shuffle(len(questions), func(n1, n2 int) {
		questions[n1], questions[n2] = questions[n2], questions[n1]
	})

	return questions
}
