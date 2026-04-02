//go:build tools

package tunnel

// This file ensures golang.org/x/mobile stays in go.mod for gomobile bind.
import _ "golang.org/x/mobile/bind"
