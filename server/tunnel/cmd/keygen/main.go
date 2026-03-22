// keygen generates X25519 key pairs for REALITY.
// Uses only the Go standard library (no external deps).
package main

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
)

func main() {
	// Generate X25519 key pair using Go's standard crypto/ecdh
	curve := ecdh.X25519()
	privateKey, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to generate key: %v\n", err)
		os.Exit(1)
	}

	privB64 := base64.RawURLEncoding.EncodeToString(privateKey.Bytes())
	pubB64 := base64.RawURLEncoding.EncodeToString(privateKey.PublicKey().Bytes())

	fmt.Printf("Private Key: %s\n", privB64)
	fmt.Printf("Public Key:  %s\n", pubB64)
	fmt.Println()
	fmt.Println("Add to tunnel config.json:")
	fmt.Printf("  \"private_key\": \"%s\"\n", privB64)
	fmt.Printf("  \"public_key\": \"%s\"\n", pubB64)
}
