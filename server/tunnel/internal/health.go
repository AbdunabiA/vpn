package internal

import (
	"encoding/json"
	"net/http"
	"runtime"
	"time"
)

// HealthResponse is returned by the /health endpoint.
type HealthResponse struct {
	Status    string    `json:"status"`
	Uptime    string    `json:"uptime"`
	Protocol  string    `json:"protocol"`
	GoVersion string    `json:"go_version"`
	NumCPU    int       `json:"num_cpu"`
	Timestamp time.Time `json:"timestamp"`
}

// DetailedHealthHandler returns a more detailed health check handler.
// Used by the infrastructure orchestrator to monitor server status.
func DetailedHealthHandler(startTime time.Time, config *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := HealthResponse{
			Status:    "healthy",
			Uptime:    time.Since(startTime).Round(time.Second).String(),
			Protocol:  config.Protocol,
			GoVersion: runtime.Version(),
			NumCPU:    runtime.NumCPU(),
			Timestamp: time.Now().UTC(),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}
