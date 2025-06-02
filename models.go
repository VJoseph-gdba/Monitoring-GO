package server

import "time"

// Structures identiques au client
type TimingMetrics struct {
	DNSLookupMs      float64 `json:"dns_lookup_ms"`
	TCPConnectMs     float64 `json:"tcp_connect_ms"`
	TLSHandshakeMs   float64 `json:"tls_handshake_ms"`
	RequestSentMs    float64 `json:"request_sent_ms"`
	FirstByteMs      float64 `json:"first_byte_ms"`
	TotalResponseMs  float64 `json:"total_response_ms"`
}

type ResponseDetails struct {
	StatusCode      int               `json:"status_code"`
	StatusText      string            `json:"status_text"`
	HeadersReceived map[string]string `json:"headers_received"`
	BodySize        int64             `json:"body_size"`
	BodyPreview     string            `json:"body_preview"`
}

type NetworkInfo struct {
	LocalIP          string `json:"local_ip"`
	RemoteIP         string `json:"remote_ip"`
	ConnectionReused bool   `json:"connection_reused"`
	ProtocolVersion  string `json:"protocol_version"`
}

type ErrorDetails struct {
	HasError     bool   `json:"has_error"`
	ErrorType    string `json:"error_type"`
	ErrorMessage string `json:"error_message"`
	RetryCount   int    `json:"retry_count"`
}

type MonitoringData struct {
	ClientID        string            `json:"client_id"`
	Timestamp       string            `json:"timestamp"`
	TargetURL       string            `json:"target_url"`
	RequestDetails  map[string]string `json:"request_details"`
	TimingMetrics   TimingMetrics     `json:"timing_metrics"`
	ResponseDetails ResponseDetails   `json:"response_details"`
	NetworkInfo     NetworkInfo       `json:"network_info"`
	ErrorDetails    ErrorDetails      `json:"error_details"`
}

// Structure pour l'affichage
type ClientStatus struct {
	ID              string
	Name            string
	TargetURL       string
	LastSeen        time.Time
	IsOnline        bool
	LastLatency     float64
	LastStatusCode  int
	SuccessRate     float64
	LastError       string
	LastErrorTime   time.Time
	TimingBreakdown TimingMetrics
	NetworkInfo     NetworkInfo
}

type DashboardData struct {
	OnlineCount    int
	OfflineCount   int
	TotalCount     int
	AverageLatency float64
	Clients        []ClientStatus
}

// APIDashboardData structure pour les données du tableau de bord envoyées via API
type APIDashboardData struct {
	OnlineCount    int          `json:"online_count"`
	OfflineCount   int          `json:"offline_count"`
	TotalCount     int          `json:"total_count"`
	AverageLatency float64      `json:"average_latency"`
	Clients        []ClientStatus `json:"clients"`
	SelectedClient *ClientStatus `json:"selected_client,omitempty"` // Omit if null
	ClientHistory  []MonitoringData `json:"client_history,omitempty"`
	ClientAnomalies []MonitoringData `json:"client_anomalies,omitempty"`
}

type HistoryFilterOptions struct {
	ClientID      string
	Duration      time.Duration
	SortBy        string // e.g., "timestamp", "latency", "status_code"
	SortOrder     string // "asc" or "desc"
	Limit         int    // Max number of results
	StatusFilter  string // "success", "error", "all"
	MinLatency    float64
	MaxLatency    float64
}