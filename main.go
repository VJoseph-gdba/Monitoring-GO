package main

import (
	"log"
	"net/http"
	"time"

	"monitoring-go/server" // Remplacez par le nom de votre module
)

func main() {
	srv, err := server.NewServer("monitor.db")
	if err != nil {
		log.Fatalf("Échec du démarrage du serveur: %v", err)
	}
	defer srv.Close()

	// Créer un mux personnalisé
	mux := http.NewServeMux()
	mux.HandleFunc("/data", srv.HandleMonitoringData)
	mux.HandleFunc("/api/dashboard_data", srv.HandleAPIDashboardData)
	mux.HandleFunc("/", srv.HandleDashboard)
	mux.HandleFunc("/api/clients", srv.HandleGetClients)

	// Serveur HTTP avec timeouts configurés
	httpServer := &http.Server{
		Addr:           ":8080",
		Handler:        mux,
		ReadTimeout:    15 * time.Second,  // Timeout pour lire la requête
		WriteTimeout:   30 * time.Second,  // Timeout pour écrire la réponse
		IdleTimeout:    60 * time.Second,  // Timeout pour les connexions inactives
		MaxHeaderBytes: 1 << 20,           // 1 MB
	}

	log.Println("Serveur démarré sur :8080")
	log.Fatal(httpServer.ListenAndServe())
}
