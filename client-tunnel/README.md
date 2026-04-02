# Client Tunnel Library

Go library that wraps xray-core to provide VLESS+REALITY tunneling for the VPN mobile app. Compiled via **gomobile** into native artifacts:

- **Android:** `tunnel.aar`
- **iOS:** `Tunnel.xcframework`

## Requirements

- **Go 1.23+** (go.mod specifies 1.25.0; Go toolchain auto-downloads it)
  - Install Go 1.23+ and the toolchain feature handles the rest
- **gomobile** + **gobind**: `make setup`
- **Android SDK** (for Android builds)
- **Xcode** (for iOS builds)

## Build

```bash
# One-time setup
make setup

# Fetch dependencies
make deps

# Run tests
make test

# Build for Android (outputs to ../app/android/app/libs/tunnel.aar)
make android

# Build for iOS device (outputs to ../app/ios/Frameworks/Tunnel.xcframework)
make ios

# Build for iOS device + simulator
make ios-sim

# Build both platforms
make all
```

## API

All exported functions use primitive types (string, int) for gomobile compatibility.

| Function | Description |
|----------|-------------|
| `Connect(configJSON string) string` | Start xray-core SOCKS5 proxy on localhost:10808 |
| `Disconnect() string` | Tear down xray-core |
| `StartTun(fd int) string` | Bridge TUN fd to SOCKS5 via tun2socks |
| `StopTun() string` | Stop tun2socks |
| `GetStatus() string` | Current tunnel state as JSON |
| `GetTrafficStats() string` | Bandwidth stats as JSON |
| `ProbeServers(serversJSON string) string` | TCP latency probing |
| `SetStatusCallback(cb StatusCallback)` | Register state change listener |
| `SetProtectCallback(cb ProtectSocket)` | Register Android socket protector |

## Architecture

```
Native VPN Service
    |
    v
StartTun(fd) --> tun2socks --> SOCKS5 (localhost:10808) --> xray-core --> VLESS+REALITY --> Remote Server
```
