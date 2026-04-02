# Client Tunnel Library — gomobile build targets
# Produces .aar (Android) and .xcframework (iOS) from Go code

.PHONY: all android ios ios-sim clean setup verify deps test

# Output paths — directly into the app project
ANDROID_OUT = ../app/android/app/libs/tunnel.aar
IOS_OUT     = ../app/ios/Frameworks/Tunnel.xcframework

# Ensure output directories exist
../app/android/app/libs:
	mkdir -p $@

../app/ios/Frameworks:
	mkdir -p $@

# Install gomobile and initialize it
setup:
	go install golang.org/x/mobile/cmd/gomobile@latest
	go install golang.org/x/mobile/cmd/gobind@latest
	gomobile init

# Verify environment before building
verify:
	@which gomobile > /dev/null || (echo "ERROR: gomobile not found. Run 'make setup'" && exit 1)
	@go build ./...
	@echo "Environment OK"

# Build for Android — produces tunnel.aar
android: verify | ../app/android/app/libs
	gomobile bind -target=android -androidapi 24 -ldflags="-s -w" -o $(ANDROID_OUT) ./
	@echo "Built $(ANDROID_OUT)"

# Build for iOS (arm64 only — device builds)
ios: verify | ../app/ios/Frameworks
	gomobile bind -target=ios/arm64 -ldflags="-s -w" -o $(IOS_OUT) ./
	@echo "Built $(IOS_OUT)"

# Build for iOS including simulator (arm64 + amd64)
ios-sim: verify | ../app/ios/Frameworks
	gomobile bind -target=ios -ldflags="-s -w" -o $(IOS_OUT) ./
	@echo "Built $(IOS_OUT) (with simulator support)"

# Build for both platforms
all: android ios

# Run Go tests
test:
	go test -v ./...

# Clean build artifacts
clean:
	rm -f tunnel.aar tunnel-sources.jar
	rm -rf Tunnel.xcframework
	rm -f $(ANDROID_OUT)
	rm -rf $(IOS_OUT)

# Fetch and tidy Go dependencies
deps:
	go mod tidy
	go mod download
