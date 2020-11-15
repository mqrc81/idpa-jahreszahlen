package mysql

import (
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/mqrc81/IDPA-Jahreszahlen/backend"
)

type TopicStore struct {
	*sqlx.DB
}

/*
 * Get topic by topic id
 */
func (s *TopicStore) Topic(topicID int) (backend.Topic, error) {
	var u backend.Topic
	if err := s.Get(&u, `SELECT * FROM topics WHERE topic_id = $1`, topicID); err != nil {
		return backend.Topic{}, fmt.Errorf("error getting topic: %w", err)
	}
	return u, nil
}

/*
 * Get topics
 */
func (s *TopicStore) Topics() ([]backend.Topic, error) {
	var uu []backend.Topic
	if err := s.Select(&uu, `SELECT * FROM topics`); err != nil {
		return []backend.Topic{}, fmt.Errorf("error getting topics: %w", err)
	}
	return uu, nil
}

/*
 * Create topic
 */
func (s *TopicStore) CreateTopic(u *backend.Topic) error {
	if _, err := s.Exec(`INSERT INTO topics(title, start_year, end_year, description) VALUES ($1, $2, $3, $4)`,
		u.Title,
		u.StartYear,
		u.EndYear,
		u.Description); err != nil {
		return fmt.Errorf("error creating topic: %w", err)
	}
	return nil
}

/*
 * Update topic
 */
func (s *TopicStore) UpdateTopic(u *backend.Topic) error {
	if _, err := s.Exec(`UPDATE topics SET title = $1, start_year = $2, end_year = $3, description = $4 WHERE topic_id = $5`,
		u.Title,
		u.StartYear,
		u.EndYear,
		u.Description,
		u.TopicID); err != nil {
		return fmt.Errorf("error updating topic: %w", err)
	}
	return nil
}

/*
 * Delete topic by topic id
 */
func (s *TopicStore) DeleteTopic(topicID int) error {
	if _, err := s.Exec(`DELETE FROM topics WHERE topic_id = $1`, topicID); err != nil {
		return fmt.Errorf("error deleting topic: %w", err)
	}
	return nil
}
