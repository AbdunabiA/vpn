package tunnel

import (
	"fmt"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/xjasonlyu/tun2socks/v2/engine"
	tun2socksLog "github.com/xjasonlyu/tun2socks/v2/log"
)

func init() {
	// Override tun2socks logger: tun2socks v2.6.0 uses zap and calls
	// log.Fatalf on error, which triggers os.Exit(1) via zap's default
	// fatal hook. We replace it with WriteThenPanic so the Go recover()
	// in StartTun() can catch it instead of killing the Android process.
	cfg := zap.NewProductionConfig()
	cfg.Level.SetLevel(zap.WarnLevel)
	logger, err := cfg.Build(zap.WithFatalHook(zapcore.WriteThenPanic))
	if err == nil {
		tun2socksLog.SetLogger(logger)
	}
}

var (
	tunMu      sync.Mutex
	tunRunning bool
)

// StartTun starts the tun2socks engine that bridges a TUN file descriptor
// to the local SOCKS5 proxy opened by Connect().
//
// This is the path for xray-based protocols ("vless-reality", "vless-ws").
// For AmneziaWG use StartTunAWG instead.
//
// fd: the TUN file descriptor from the OS VPN service
//   - Android: VpnService.Builder.establish().getFd()
//   - iOS: socketpair fd bridged to NEPacketTunnelFlow
//
// Returns empty string on success, error message on failure.
// Must be called AFTER Connect() has successfully started xray-core.
func StartTun(fd int) (result string) {
	defer func() {
		if r := recover(); r != nil {
			result = fmt.Sprintf("PANIC in StartTun: %v", r)
		}
	}()

	tunMu.Lock()
	defer tunMu.Unlock()

	// If tun2socks is already running, stop it first before restarting.
	// This prevents deadlocks from engine's internal mutex and avoids
	// log.Fatalf crashes when the engine is re-entered.
	if tunRunning {
		engine.Stop()
		tunRunning = false
	}

	key := new(engine.Key)
	key.Proxy = socksProxyURL()
	key.Device = fmt.Sprintf("fd://%d", fd)
	key.LogLevel = "silent"
	key.MTU = 1400
	key.TCPModerateReceiveBuffer = true
	key.TCPSendBufferSize = "256KB"
	key.TCPReceiveBufferSize = "256KB"

	engine.Insert(key)
	engine.Start()

	tunRunning = true
	return ""
}

// StopTun stops the tun2socks engine.
// Should be called before Disconnect() to cleanly shut down.
func StopTun() string {
	tunMu.Lock()
	defer tunMu.Unlock()

	if !tunRunning {
		return ""
	}

	engine.Stop()
	tunRunning = false
	return ""
}

// StartTunAWG hands the TUN file descriptor directly to the AmneziaWG device.
//
// This is the correct path when Protocol is "amneziawg".  Unlike StartTun,
// there is no tun2socks or SOCKS5 proxy involved: the WireGuard device reads
// and writes raw IP packets on the TUN fd itself.
//
// The AWG configuration is retrieved from the pending slot set by Connect()
// when the "amneziawg" protocol was requested.  Therefore StartTunAWG must
// be called AFTER Connect() returns successfully.
//
// fd: the TUN file descriptor from the OS VPN service (same as StartTun).
//
// Returns empty string on success, error message on failure.
func StartTunAWG(fd int) string {
	cfg := takePendingAWGConfig()
	if cfg == nil {
		return "no pending awg config: call Connect() with protocol=amneziawg first"
	}

	if err := startAWGTunnel(fd, *cfg); err != nil {
		return fmt.Sprintf("awg tunnel start error: %v", err)
	}

	return ""
}
