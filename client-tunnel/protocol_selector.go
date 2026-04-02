package tunnel

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"
)

// ProbeResult holds the result of probing a single server with a specific protocol.
type ProbeResult struct {
	ServerAddress string `json:"server_address"`
	Protocol      string `json:"protocol"`
	Latency       int64  `json:"latency_ms"` // Round-trip time in milliseconds
	Success       bool   `json:"success"`
	Error         string `json:"error,omitempty"`
}

// ProbeServers tests connectivity to multiple servers in parallel
// and returns results sorted by latency (fastest first).
//
// This is called by the React Native app's protocol prober service
// to determine which protocol and server combination works best
// for the user's current network conditions.
//
// serversJSON: JSON array of server objects with "address" and "port" fields
// Returns: JSON array of ProbeResult objects
func ProbeServers(serversJSON string) string {
	// Probes make direct TCP connections that bypass the tunnel.
	// Block probing while connected to prevent real IP leaks.
	mgr := getManager()
	mgr.mu.Lock()
	state := mgr.status.State
	mgr.mu.Unlock()
	if state == StateConnected || state == StateConnecting {
		return `[{"error": "cannot probe while VPN is active — probes bypass the tunnel"}]`
	}

	type serverEntry struct {
		Address string `json:"address"`
		Port    int    `json:"port"`
	}

	var servers []serverEntry
	if err := json.Unmarshal([]byte(serversJSON), &servers); err != nil {
		return fmt.Sprintf(`[{"error": "invalid input: %v"}]`, err)
	}

	var wg sync.WaitGroup
	results := make([]ProbeResult, len(servers))

	for i, srv := range servers {
		wg.Add(1)
		go func(idx int, s serverEntry) {
			defer wg.Done()
			results[idx] = probeServer(s.Address, s.Port)
		}(i, srv)
	}

	wg.Wait()

	// Sort by latency (successful probes first, then by latency)
	sortProbeResults(results)

	data, _ := json.Marshal(results)
	return string(data)
}

// probeServer tests TCP connectivity to a single server.
// For REALITY servers, this is a TCP connection to port 443 — the same port
// that hosts real HTTPS, so the probe itself looks like a normal web request.
func probeServer(address string, port int) ProbeResult {
	result := ProbeResult{
		ServerAddress: fmt.Sprintf("%s:%d", address, port),
		Protocol:      "vless-reality",
	}

	start := time.Now()

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", address, port), 5*time.Second)
	if err != nil {
		result.Success = false
		result.Error = err.Error()
		return result
	}
	defer conn.Close()

	result.Latency = time.Since(start).Milliseconds()
	result.Success = true
	return result
}

// sortProbeResults sorts results: successful first (by latency), then failed.
func sortProbeResults(results []ProbeResult) {
	n := len(results)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			swap := false
			if !results[j].Success && results[j+1].Success {
				swap = true
			} else if results[j].Success && results[j+1].Success && results[j].Latency > results[j+1].Latency {
				swap = true
			}
			if swap {
				results[j], results[j+1] = results[j+1], results[j]
			}
		}
	}
}
