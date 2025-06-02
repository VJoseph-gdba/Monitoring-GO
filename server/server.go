package server

import (
	"database/sql"
	"log"
	"time"
)

// Server structure holds the database connection and methods.
type Server struct {
	db *sql.DB
}

// NewServer creates a new Server instance, initializes the database, and starts cleanup.
func NewServer(dbPath string) (*Server, error) {
	db, err := initDatabase(dbPath)
	if err != nil {
		return nil, err
	}

	s := &Server{
		db: db,
	}

	// Start the cleanup routine in a goroutine
	go s.cleanupRoutine()

	return s, nil
}

// Close closes the database connection.
func (s *Server) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// cleanupRoutine periodically deletes old client history data.
func (s *Server) cleanupRoutine() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		// Delete data older than 7 days
		_, err := s.db.Exec(`
			DELETE FROM client_history
			WHERE timestamp < datetime('now', '-7 days')`)

		if err != nil {
			log.Printf("Erreur nettoyage base: %v", err)
		} else {
			log.Println("Nettoyage automatique des anciennes données effectué")
		}
	}
}