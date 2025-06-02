package server

import (
	"context"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// HandleMonitoringData receives monitoring data from clients.
func (s *Server) HandleMonitoringData(w http.ResponseWriter, r *http.Request) {
	// Ajouter un timeout pour éviter les connexions qui traînent
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	r = r.WithContext(ctx)

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var data MonitoringData
	err := json.NewDecoder(r.Body).Decode(&data)
	if err != nil {
		log.Printf("Erreur décodage JSON: %v", err)
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Réponse immédiate pour éviter les timeouts
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))

	// Flush la réponse immédiatement
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	// Traiter les données en arrière-plan
	go func() {
		err = s.storeMonitoringData(data)
		if err != nil {
			log.Printf("Erreur de stockage des données de monitoring: %v", err)
			return
		}

		status := "✓"
		if data.ErrorDetails.HasError {
			status = "✗"
			log.Printf("[%s] %s %s - Erreur: %s",
				time.Now().Format("15:04:05"), status, data.ClientID, data.ErrorDetails.ErrorType)
		} else {
			log.Printf("[%s] %s %s - %dms (Statut: %d)",
				time.Now().Format("15:04:05"), status, data.ClientID,
				int(data.TimingMetrics.TotalResponseMs), data.ResponseDetails.StatusCode)
		}
	}()
}

// HandleDashboard renders the main dashboard HTML page.
func (s *Server) HandleDashboard(w http.ResponseWriter, r *http.Request) {
	// Ajouter un timeout pour le dashboard
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	r = r.WithContext(ctx)

	// Vérifier si le client a fermé la connexion
	select {
	case <-ctx.Done():
		log.Printf("Connexion fermée par le client avant traitement du dashboard")
		return
	default:
	}

	clients, err := s.getClientStatuses()
	if err != nil {
		log.Printf("Erreur récupération données clients: %v", err)
		http.Error(w, "Erreur de récupération des données", http.StatusInternalServerError)
		return
	}

	// Vérifier à nouveau si la connexion est toujours active
	select {
	case <-ctx.Done():
		log.Printf("Connexion fermée par le client pendant récupération des données")
		return
	default:
	}

	sort.Slice(clients, func(i, j int) bool {
		if clients[i].IsOnline != clients[j].IsOnline {
			return clients[i].IsOnline
		}
		return clients[i].Name < clients[j].Name
	})

	onlineCount := 0
	totalLatency := 0.0
	validLatencyCount := 0

	for _, client := range clients {
		if client.IsOnline {
			onlineCount++
		}
		if client.LastLatency > 0 {
			totalLatency += client.LastLatency
			validLatencyCount++
		}
	}

	avgLatency := 0.0
	if validLatencyCount > 0 {
		avgLatency = totalLatency / float64(validLatencyCount)
	}

	selectedClientID := r.URL.Query().Get("client")
	selectedDurationStr := r.URL.Query().Get("duration")
	if selectedDurationStr == "" {
		selectedDurationStr = "1h"
	}

	duration, errParseDuration := time.ParseDuration(selectedDurationStr)
	if errParseDuration != nil {
		log.Printf("Erreur de parsing de la durée '%s': %v, utilisation par défaut 1h", selectedDurationStr, errParseDuration)
		duration = 1 * time.Hour
	}

	sortBy := r.URL.Query().Get("sort_by")
	if sortBy == "" {
		sortBy = "timestamp"
	}
	sortOrder := r.URL.Query().Get("sort_order")
	if sortOrder == "" {
		sortOrder = "desc"
	}
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if l, parseErr := strconv.Atoi(limitStr); parseErr == nil && l > 0 {
			limit = l
		}
	}
	statusFilter := r.URL.Query().Get("status_filter")
	if statusFilter == "" {
		statusFilter = "all"
	}

	minLatencyStr := r.URL.Query().Get("min_latency")
	minLatency := 0.0
	if minLatencyStr != "" {
		if ml, parseErr := strconv.ParseFloat(minLatencyStr, 64); parseErr == nil && ml >= 0 {
			minLatency = ml
		}
	}
	maxLatencyStr := r.URL.Query().Get("max_latency")
	maxLatency := 0.0
	if maxLatencyStr != "" {
		if ml, parseErr := strconv.ParseFloat(maxLatencyStr, 64); parseErr == nil && ml >= 0 {
			maxLatency = ml
		}
	}

	var selectedClient *ClientStatus
	var clientHistory []MonitoringData
	var clientAnomalies []MonitoringData

	if selectedClientID != "" {
		for i := range clients {
			if clients[i].ID == selectedClientID {
				selectedClient = &clients[i]
				break
			}
		}

		if selectedClient != nil {
			filterOptions := HistoryFilterOptions{
				ClientID:     selectedClientID,
				Duration:     duration,
				SortBy:       sortBy,
				SortOrder:    sortOrder,
				Limit:        limit,
				StatusFilter: statusFilter,
				MinLatency:   minLatency,
				MaxLatency:   maxLatency,
			}

			var errHistory error
			clientHistory, errHistory = s.getFilteredClientHistory(filterOptions)
			if errHistory != nil {
				log.Printf("Error getting filtered client history for client %s: %v", selectedClientID, errHistory)
				// Not returning here, just logging the error. clientHistory might be partially filled or nil.
			}

			var errAnomalies error
			clientAnomalies, errAnomalies = s.getAnomalies(selectedClientID, 1000.0, duration, 100)
			if errAnomalies != nil {
				log.Printf("Error getting anomalies for client %s: %v", selectedClientID, errAnomalies)
				// Not returning here, just logging the error. clientAnomalies might be partially filled or nil.
			}
		}
	}

	// Vérifier une dernière fois avant de rendre le template
	select {
	case <-ctx.Done():
		log.Printf("Connexion fermée par le client avant rendu du template")
		return
	default:
	}

	pageData := struct {
		DashboardData
		SelectedClient      *ClientStatus
		ClientHistory       []MonitoringData
		ClientAnomalies     []MonitoringData
		SelectedDuration    string
		AvailableDurations  map[string]string
		CurrentSortBy       string
		CurrentSortOrder    string
		CurrentLimit        int
		CurrentStatusFilter string
		CurrentMinLatency   float64
		CurrentMaxLatency   float64
	}{
		DashboardData: DashboardData{
			OnlineCount:    onlineCount,
			OfflineCount:   len(clients) - onlineCount,
			TotalCount:     len(clients),
			AverageLatency: avgLatency,
			Clients:        clients,
		},
		SelectedClient:      selectedClient,
		ClientHistory:       clientHistory,
		ClientAnomalies:     clientAnomalies,
		SelectedDuration:    selectedDurationStr,
		AvailableDurations: map[string]string{"1h": "1 heure", "6h": "6 heures", "24h": "24 heures", "7d": "7 jours", "30d": "30 jours"},
		CurrentSortBy:       sortBy,
		CurrentSortOrder:    sortOrder,
		CurrentLimit:        limit,
		CurrentStatusFilter: statusFilter,
		CurrentMinLatency:   minLatency,
		CurrentMaxLatency:   maxLatency,
	}

	// Set headers appropriés
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

	if err := s.dashboardTmpl.Execute(w, pageData); err != nil {
		// Vérifier si l'erreur est due à une connexion fermée
		if strings.Contains(err.Error(), "wsasend") || strings.Contains(err.Error(), "broken pipe") {
			log.Printf("Connexion fermée par le client pendant rendu: %v", err)
		} else {
			log.Printf("Erreur lors de l'exécution du template: %v", err)
		}
		return
	}
}

// HandleAPIDashboardData provides a JSON API for dashboard data.
// TODO: Implement actual data fetching and filtering logic.
func (s *Server) HandleAPIDashboardData(w http.ResponseWriter, r *http.Request) {
	log.Printf("HandleAPIDashboardData called for path: %s", r.URL.Path)

	// Example: Return a simple JSON response
	// In a real application, you would fetch data similar to HandleDashboard,
	// but format it as JSON.
	data := map[string]interface{}{
		"message": "API endpoint for dashboard data is under construction.",
		"data":    nil, // Placeholder for actual data
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error encoding JSON response for HandleAPIDashboardData: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// HandleGetClients fetches all clients from the database and returns them as JSON.
func (s *Server) HandleGetClients(w http.ResponseWriter, r *http.Request) {
	clients, err := s.getClients(r.Context()) // Assuming getClients might need context
	if err != nil {
		log.Printf("Erreur récupération des clients depuis la base de données: %v", err)
		http.Error(w, "Erreur interne du serveur lors de la récupération des clients", http.StatusInternalServerError)
		return
	}

	jsonData, err := json.Marshal(clients)
	if err != nil {
		log.Printf("Erreur lors du marshaling JSON des clients: %v", err)
		http.Error(w, "Erreur interne du serveur lors de la préparation de la réponse", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(jsonData)
	if err != nil {
		log.Printf("Erreur lors de l'écriture de la réponse JSON des clients: %v", err)
		// It's often too late to send an HTTP error if headers have been written
	}
}
