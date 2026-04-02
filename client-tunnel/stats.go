package tunnel

import (
	"encoding/json"
	"sync/atomic"
	"time"
)

// TrafficStats tracks bandwidth usage for the current session.
// Updated by the tunnel and read by the native module for UI display.
type TrafficStats struct {
	bytesUp   atomic.Int64
	bytesDown atomic.Int64
	startTime time.Time
}

// StatsSnapshot is a JSON-serializable snapshot of current traffic stats.
type StatsSnapshot struct {
	BytesUp       int64   `json:"bytes_up"`
	BytesDown     int64   `json:"bytes_down"`
	SpeedUp       float64 `json:"speed_up_bps"`   // bytes per second (upload)
	SpeedDown     float64 `json:"speed_down_bps"`  // bytes per second (download)
	DurationSecs  float64 `json:"duration_secs"`
}

// NewTrafficStats creates a new traffic stats tracker.
func NewTrafficStats() *TrafficStats {
	return &TrafficStats{
		startTime: time.Now(),
	}
}

// AddUpload records uploaded bytes.
func (s *TrafficStats) AddUpload(bytes int64) {
	s.bytesUp.Add(bytes)
}

// AddDownload records downloaded bytes.
func (s *TrafficStats) AddDownload(bytes int64) {
	s.bytesDown.Add(bytes)
}

// Snapshot returns the current stats as a JSON string.
// Called by the native module to update the UI speed indicators.
func (s *TrafficStats) Snapshot() string {
	elapsed := time.Since(s.startTime).Seconds()
	up := s.bytesUp.Load()
	down := s.bytesDown.Load()

	snap := StatsSnapshot{
		BytesUp:      up,
		BytesDown:    down,
		DurationSecs: elapsed,
	}

	if elapsed > 0 {
		snap.SpeedUp = float64(up) / elapsed
		snap.SpeedDown = float64(down) / elapsed
	}

	data, _ := json.Marshal(snap)
	return string(data)
}

// GetTrafficStats returns current session traffic stats as JSON.
// Exported for gomobile — called by the native module.
func GetTrafficStats() string {
	mgr := getManager()
	mgr.mu.Lock()
	stats := mgr.stats
	mgr.mu.Unlock()

	if stats != nil {
		return stats.Snapshot()
	}

	// Return zeros when not connected
	snap := StatsSnapshot{}
	data, _ := json.Marshal(snap)
	return string(data)
}
