package server

import (
	"context" // Added
	"database/sql"
	"fmt" // Added
	"html/template"
	"log"
	"strings"
	"time"
)

// Server structure holds the database connection and methods.
type Server struct {
	db            *sql.DB
	dashboardTmpl *template.Template
}

// NewServer creates a new Server instance, initializes the database, and starts cleanup.
func NewServer(dbPath string) (*Server, error) {
	db, err := initDatabase(dbPath)
	if err != nil {
		return nil, fmt.Errorf("initializing database: %w", err)
	}

	// Define template functions
	funcMap := template.FuncMap{
		"add": func(a ...float64) float64 {
			sum := 0.0
			for _, val := range a {
				sum += val
			}
			return sum
		},
		"div": func(a, b float64) float64 {
			if b == 0 {
				return 0
			}
			return a / b
		},
		"sub": func(a, b float64) float64 { return a - b },
		"mul": func(a, b float64) float64 { return a * b },
		"max": func(a, b float64) float64 {
			if a > b {
				return a
			}
			return b
		},
		"timeFormat": func(t time.Time) string {
			return t.Format("02/01 15:04:05")
		},
		"formatTime": func(t time.Time) string {
			return t.Format("02/01/2006 15:04:05")
		},
		"formatDuration": func(d time.Duration) string {
			return d.Round(time.Second).String()
		},
		"eq": func(a, b interface{}) bool {
			return a == b
		},
		"ne": func(a, b interface{}) bool {
			return a != b
		},
		"trimSuffix": func(s, suffix string) string {
			return strings.TrimSuffix(s, suffix)
		},
		"parseTime": func(s string) time.Time {
			t, pErr := time.Parse(time.RFC3339, s)
			if pErr != nil {
				log.Printf("Erreur de parsing de la chaîne de temps '%s': %v", s, pErr)
				return time.Time{}
			}
			return t
		},
	}

	// Parse the dashboard template
	tmpl, err := template.New("dashboard.html").Funcs(funcMap).ParseFiles("templates/dashboard.html")
	if err != nil {
		return nil, fmt.Errorf("parsing dashboard template: %w", err)
	}

	s := &Server{
		db:            db,
		dashboardTmpl: tmpl,
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
