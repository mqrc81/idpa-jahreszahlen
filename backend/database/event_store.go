// The database store evolving around events, with all necessary methods that
// access the database.

package database

import (
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/mqrc81/IDPA-Jahreszahlen/backend/jahreszahlen"
)

// EventStore is the MySQL database access object.
type EventStore struct {
	*sqlx.DB
}

// GetEvent gets event by ID.
func (store *EventStore) GetEvent(eventID int) (jahreszahlen.Event, error) {
	var event jahreszahlen.Event

	// Execute prepared statement
	query := `
		SELECT *
		FROM events
		WHERE event_id = ?
		`
	if err := store.Get(&event, query, eventID); err != nil {
		return jahreszahlen.Event{}, fmt.Errorf("error getting event: %w", err)
	}

	return event, nil
}

// CountEvents gets amount of events.
func (store *EventStore) CountEvents() (int, error) {
	var eventCount int

	// Execute prepared statement
	query := `
		SELECT COUNT(*) 
		FROM events
		`
	if err := store.Get(&eventCount, query); err != nil {
		return 0, fmt.Errorf("error getting number of events: %w", err)
	}

	return eventCount, nil
}

// CreateEvent creates a new event.
func (store *EventStore) CreateEvent(event *jahreszahlen.Event) error {

	// Execute prepared statement
	query := `
		INSERT INTO events(topic_id, name, year, date) 
		VALUES (?, ?, ?, ?)
		`
	if _, err := store.Exec(query,
		event.TopicID,
		event.Name,
		event.Year,
		event.Year); err != nil {
		return fmt.Errorf("error creating event: %w", err)
	}

	return nil
}

// UpdateEvent updates an existing event.
func (store *EventStore) UpdateEvent(event *jahreszahlen.Event) error {

	// Execute prepared statement
	query := `
		UPDATE events 
		SET name = ?, 
		    year = ?,
		    date = ?
		WHERE event_id = ?
		`
	if _, err := store.Exec(query,
		event.Name,
		event.Year,
		event.Date,
		event.EventID); err != nil {
		return fmt.Errorf("error updating event: %w", err)
	}

	return nil
}

// DeleteEvent deletes an existing event.
func (store *EventStore) DeleteEvent(eventID int) error {

	// Execute prepared statement
	query := `
		DELETE FROM events 
		WHERE event_id = ?
		`
	if _, err := store.Exec(query, eventID); err != nil {
		return fmt.Errorf("error deleting event: %w", err)
	}

	return nil
}
