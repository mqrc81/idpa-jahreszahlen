package web

/*
 * sessions.go adds sessions-management
 */

import (
	"database/sql"

	"github.com/alexedwards/scs/mysqlstore"
	"github.com/alexedwards/scs/v2"
)

/*
 * NewSessionManager creates new session
 */
func NewSessionManager(dataSourceName string) (*scs.SessionManager, error) {
	db, err := sql.Open("mysql", dataSourceName)
	if err != nil {
		return nil, err
	}

	sessions := scs.New()
	sessions.Store = mysqlstore.New(db)
	return sessions, nil
}
