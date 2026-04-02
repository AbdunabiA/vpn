//go:build !linux

package tunnel

import "fmt"

// startAWGTunnel is a stub for non-Linux platforms.
//
// The real implementation lives in awg_linux.go.  This stub exists so the
// package compiles on macOS (developer machines) and allows running the unit
// tests that do not exercise the actual device startup path.
//
// gomobile produces only Android (Linux) and iOS builds.  iOS uses a
// packet-tunnel provider that also relies on the Linux code path when running
// on a real device, but for simulator builds this stub prevents compile errors.
func startAWGTunnel(_ int, _ AWGConfig) error {
	return fmt.Errorf("startAWGTunnel: not supported on this platform (requires Linux/Android)")
}
