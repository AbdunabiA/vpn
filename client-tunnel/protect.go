package tunnel

import (
	"fmt"
	"sync"
	"syscall"

	"github.com/xtls/xray-core/transport/internet"
)

// ProtectSocket is implemented by the Android VpnService to prevent
// the tunnel's own sockets from being routed through the VPN (routing loop).
//
// gomobile generates a Java interface: tunnel.ProtectSocket
// The Android TunnelVpnService implements it by calling VpnService.protect(fd).
//
// On iOS this is not needed — the Network Extension process is exempt
// from its own tunnel by default.
type ProtectSocket interface {
	Protect(fd int) bool
}

var (
	protectMu       sync.RWMutex
	protectCallback ProtectSocket
	dialerOnce      sync.Once
)

// SetProtectCallback registers the socket protection callback.
// Must be called before Connect() on Android. No-op on iOS.
func SetProtectCallback(cb ProtectSocket) {
	protectMu.Lock()
	protectCallback = cb
	protectMu.Unlock()
}

// registerDialerController hooks into xray-core's socket creation to call
// the Android VpnService.protect() on each new outbound socket.
// Registered once — subsequent calls are no-ops.
func registerDialerController() {
	protectMu.RLock()
	cb := protectCallback
	protectMu.RUnlock()

	if cb == nil {
		return
	}

	dialerOnce.Do(func() {
		internet.RegisterDialerController(func(network, address string, conn syscall.RawConn) error {
			protectMu.RLock()
			currentCb := protectCallback
			protectMu.RUnlock()

			if currentCb == nil {
				return nil
			}
			var protectErr error
			_ = conn.Control(func(fd uintptr) {
				if !currentCb.Protect(int(fd)) {
					protectErr = fmt.Errorf("failed to protect socket fd %d", fd)
				}
			})
			return protectErr
		})
	})
}
