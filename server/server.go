package server

import (
	"context" // Added
	"database/sql"
	"fmt" // Added
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

// getClients retrieves all clients from the database.
func (s *Server) getClients(ctx context.Context) ([]Client, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, name, target_url FROM clients ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("querying clients: %w", err)
	}
	defer rows.Close()

	var clients []Client
	for rows.Next() {
		var c Client
		if err := rows.Scan(&c.ID, &c.Name, &c.TargetURL); err != nil {
			log.Printf("Error scanning client row: %v", err) // Log and continue for now
			continue
		}
		clients = append(clients, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("processing client rows: %w", err)
	}
	return clients, nil
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
