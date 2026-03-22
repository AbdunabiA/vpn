# Client Tunnel Library — gomobile build targets
# Produces .aar (Android) and .xcframework (iOS) from Go code

.PHONY: all android ios clean setup

# Install gomobile and initialize it
setup:
	go install golang.org/x/mobile/cmd/gomobile@latest
	gomobile init

# Build for Android — produces tunnel.aar
android:
	gomobile bind -target=android -o tunnel.aar -androidapi 24 ./

# Build for iOS — produces Tunnel.xcframework
ios:
	gomobile bind -target=ios -o Tunnel.xcframework ./

# Build for both platforms
all: android ios

# Clean build artifacts
clean:
	rm -f tunnel.aar tunnel-sources.jar
	rm -rf Tunnel.xcframework

# Fetch Go dependencies
deps:
	go mod tidy
	go mod download
