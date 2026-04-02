package tunnel

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"
)

// speedWindow is the interval at which the sliding-window baseline is refreshed.
const speedWindow = 2 * time.Second

// TrafficStats tracks bandwidth usage for the current session.
// Updated by the tunnel and read by the native module for UI display.
//
// Speed is computed as a sliding window (current_bytes - prev_bytes) /
// (current_time - prev_time) so that the UI shows instantaneous throughput
// rather than the session average.  The window baseline is refreshed every
// speedWindow seconds.
type TrafficStats struct {
	bytesUp   atomic.Int64
	bytesDown atomic.Int64
	startTime time.Time

	// Sliding-window baseline — protected by mu.
	mu           sync.Mutex
	prevUp       int64
	prevDown     int64
	prevTime     time.Time
}

// StatsSnapshot is a JSON-serializable snapshot of current traffic stats.
type StatsSnapshot struct {
	BytesUp      int64   `json:"bytes_up"`
	BytesDown    int64   `json:"bytes_down"`
	SpeedUp      float64 `json:"speed_up_bps"`  // bytes per second (upload, sliding window)
	SpeedDown    float64 `json:"speed_down_bps"` // bytes per second (download, sliding window)
	DurationSecs float64 `json:"duration_secs"`
}

// NewTrafficStats creates a new traffic stats tracker.
func NewTrafficStats() *TrafficStats {
	now := time.Now()
	return &TrafficStats{
		startTime: now,
		prevTime:  now,
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
//
// Speed is calculated over a sliding window: when speedWindow has elapsed
// since the last snapshot baseline, the previous values are advanced to the
// current ones and the window resets.  This prevents the speed display from
// showing a diminishing average over a long session.
func (s *TrafficStats) Snapshot() string {
	now := time.Now()
	elapsed := now.Sub(s.startTime).Seconds()
	up := s.bytesUp.Load()
	down := s.bytesDown.Load()

	s.mu.Lock()
	windowSecs := now.Sub(s.prevTime).Seconds()

	var speedUp, speedDown float64
	if windowSecs > 0 {
		speedUp = float64(up-s.prevUp) / windowSecs
		speedDown = float64(down-s.prevDown) / windowSecs
	}

	// Advance the baseline once the window has elapsed.
	if now.Sub(s.prevTime) >= speedWindow {
		s.prevUp = up
		s.prevDown = down
		s.prevTime = now
	}
	s.mu.Unlock()

	snap := StatsSnapshot{
		BytesUp:      up,
		BytesDown:    down,
		SpeedUp:      speedUp,
		SpeedDown:    speedDown,
		DurationSecs: elapsed,
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
