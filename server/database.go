package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// initDatabase initializes the SQLite database and creates necessary tables.
func initDatabase(dataSourceName string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dataSourceName)
	if err != nil {
		return nil, err
	}

	// Create tables
	schema := `
	CREATE TABLE IF NOT EXISTS clients (
		id TEXT PRIMARY KEY,
		name TEXT,
		target_url TEXT,
		last_seen DATETIME,
		last_data TEXT
	);

	CREATE TABLE IF NOT EXISTS client_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		client_id TEXT,
		timestamp DATETIME,
		success BOOLEAN,
		latency REAL,
		status_code INTEGER,
		error_type TEXT,
		data TEXT,
		FOREIGN KEY(client_id) REFERENCES clients(id)
	);

	CREATE INDEX IF NOT EXISTS idx_client_history_client_time
	ON client_history(client_id, timestamp DESC);
	`

	_, err = db.Exec(schema)
	return db, err
}

// storeMonitoringData stores monitoring data into the database.
func (s *Server) storeMonitoringData(data MonitoringData) error {
	// Serialize the complete data
	jsonData, _ := json.Marshal(data)

	// Update client's last seen and last data
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO clients (id, name, target_url, last_seen, last_data)
		VALUES (?, ?, ?, ?, ?)`,
		data.ClientID, data.ClientID, data.TargetURL, time.Now(), string(jsonData))

	if err != nil {
		return err
	}

	// Add to history
	success := !data.ErrorDetails.HasError
	latency := data.TimingMetrics.TotalResponseMs
	statusCode := data.ResponseDetails.StatusCode
	errorType := ""
	if data.ErrorDetails.HasError {
		errorType = data.ErrorDetails.ErrorType
	}

	_, err = s.db.Exec(`
		INSERT INTO client_history (client_id, timestamp, success, latency, status_code, error_type, data)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		data.ClientID, time.Now(), success, latency, statusCode, errorType, string(jsonData))

	return err
}

// getFilteredClientHistory retrieves filtered history data for a given client.
func (s *Server) getFilteredClientHistory(options HistoryFilterOptions) ([]MonitoringData, error) {
	var history []MonitoringData
	var args []interface{}

	query := `
		SELECT data
		FROM client_history
		WHERE client_id = ? AND timestamp > ?`
	args = append(args, options.ClientID, time.Now().Add(-options.Duration))

	if options.StatusFilter == "success" {
		query += ` AND success = 1`
	} else if options.StatusFilter == "error" {
		query += ` AND success = 0`
	}

	if options.MinLatency > 0 {
		query += ` AND latency >= ?`
		args = append(args, options.MinLatency)
	}
	if options.MaxLatency > 0 {
		query += ` AND latency <= ?`
		args = append(args, options.MaxLatency)
	}

	orderBy := "timestamp"
	switch options.SortBy {
	case "latency":
		orderBy = "latency"
	case "status_code":
		orderBy = "status_code"
	case "error_type":
		orderBy = "error_type"
	}
	query += fmt.Sprintf(` ORDER BY %s`, orderBy)

	if options.SortOrder == "desc" {
		query += ` DESC`
	} else {
		query += ` ASC`
	}

	if options.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, options.Limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var dataStr string
		if err := rows.Scan(&dataStr); err != nil {
			log.Printf("Erreur de scan de l'historique client (filtre) pour le client %s: %v", options.ClientID, err)
			continue
		}
		var data MonitoringData
		if err := json.Unmarshal([]byte(dataStr), &data); err != nil {
			log.Printf("Erreur de décodage JSON de l'historique client (filtre) pour le client %s: %v", options.ClientID, err)
			continue
		}
		history = append(history, data)
	}
	return history, nil
}

// getClientStatuses retrieves the current status of all clients.
func (s *Server) getClientStatuses() ([]ClientStatus, error) {
	rows, err := s.db.Query(`
		SELECT id, name, target_url, last_seen, last_data
		FROM clients
		ORDER BY last_seen DESC`)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var clients []ClientStatus
	now := time.Now()

	for rows.Next() {
		var id, name, targetURL, lastDataStr string
		var lastSeen time.Time

		err := rows.Scan(&id, &name, &targetURL, &lastSeen, &lastDataStr)
		if err != nil {
			log.Printf("Erreur de scan de la ligne client: %v", err)
			continue
		}

		var lastData MonitoringData
		json.Unmarshal([]byte(lastDataStr), &lastData) // Errors here are non-fatal, as we have fallback data

		successRate := s.calculateSuccessRate(id)
		lastError, lastErrorTime := s.getLastError(id)

		client := ClientStatus{
			ID:              id,
			Name:            name,
			TargetURL:       targetURL,
			LastSeen:        lastSeen,
			IsOnline:        now.Sub(lastSeen) < 60*time.Second, // Offline after 1 minute
			LastLatency:     lastData.TimingMetrics.TotalResponseMs,
			LastStatusCode:  lastData.ResponseDetails.StatusCode,
			SuccessRate:     successRate,
			LastError:       lastError,
			LastErrorTime:   lastErrorTime,
			TimingBreakdown: lastData.TimingMetrics,
			NetworkInfo:     lastData.NetworkInfo,
		}

		clients = append(clients, client)
	}

	return clients, nil
}

// calculateSuccessRate calculates the success rate for a client over the last 24 hours.
func (s *Server) calculateSuccessRate(clientID string) float64 {
	var total, success int

	err := s.db.QueryRow(`
		SELECT COUNT(*), SUM(CASE WHEN success THEN 1 ELSE 0 END)
		FROM client_history
		WHERE client_id = ? AND timestamp > datetime('now', '-24 hours')`,
		clientID).Scan(&total, &success)

	if err != nil || total == 0 {
		return 0.0
	}

	return (float64(success) / float64(total)) * 100.0
}

// getLastError retrieves the last error for a given client.
func (s *Server) getLastError(clientID string) (string, time.Time) {
	var errorType string
	var timestamp time.Time

	err := s.db.QueryRow(`
		SELECT error_type, timestamp
		FROM client_history
		WHERE client_id = ? AND success = 0
		ORDER BY timestamp DESC LIMIT 1`,
		clientID).Scan(&errorType, &timestamp)

	if err != nil {
		return "", time.Time{}
	}

	return errorType, timestamp
}

// getClientHistory retrieves history data for a client over a specified duration.
func (s *Server) getClientHistory(clientID string, duration time.Duration) ([]MonitoringData, error) {
	var history []MonitoringData
	rows, err := s.db.Query(`
		SELECT data
		FROM client_history
		WHERE client_id = ? AND timestamp > ?
		ORDER BY timestamp ASC`,
		clientID, time.Now().Add(-duration))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var dataStr string
		if err := rows.Scan(&dataStr); err != nil {
			log.Printf("Erreur de scan de l'historique client pour le client %s: %v", clientID, err)
			continue
		}
		var data MonitoringData
		if err := json.Unmarshal([]byte(dataStr), &data); err != nil {
			log.Printf("Erreur de décodage JSON de l'historique client pour le client %s: %v", clientID, err)
			continue
		}
		history = append(history, data)
	}
	return history, nil
}

// getAnomalies retrieves history entries where latency exceeds a threshold or an error occurred.
func (s *Server) getAnomalies(clientID string, thresholdMs float64, duration time.Duration, limit int) ([]MonitoringData, error) {
	var anomalies []MonitoringData
	query := `
		SELECT data
		FROM client_history
		WHERE client_id = ? AND (success = 0 OR latency > ?) AND timestamp > ?
		ORDER BY timestamp DESC`

	args := []interface{}{clientID, thresholdMs, time.Now().Add(-duration)}

	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var dataStr string
		if err := rows.Scan(&dataStr); err != nil {
			log.Printf("Erreur de scan des anomalies pour le client %s: %v", clientID, err)
			continue
		}
		var data MonitoringData
		if err := json.Unmarshal([]byte(dataStr), &data); err != nil {
			log.Printf("Erreur de décodage JSON pour les anomalies pour le client %s: %v", clientID, err)
			continue
		}
		anomalies = append(anomalies, data)
	}
	return anomalies, nil
}