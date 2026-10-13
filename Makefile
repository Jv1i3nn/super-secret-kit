.PHONY: build test install clean lint lint-fix run cross-compile help \
	build-deb build-deb-arm64 build-deb-armv6 clean-deb

BINARY_NAME=ssk
BUILD_DIR=bin
MAIN_PATH=main.go

# A release is identified by its Git tag.  Outside a release checkout, keep
# enough Git context to distinguish development builds and local changes.
# CI can override this with VERSION=<tag> for release artifacts.
VERSION ?= $(shell git describe --tags --match '[0-9]*.[0-9]*.[0-9]*' --always --dirty 2>/dev/null || echo "dev")

GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOINSTALL=$(GOCMD) install
GOCLEAN=$(GOCMD) clean
GOMOD=$(GOCMD) mod
GOLANGCI_LINT=$(shell which golangci-lint 2>/dev/null || echo "$$HOME/go/bin/golangci-lint")

LDFLAGS=-s -w -X ssk/internal/version.Version=$(VERSION)
CGO_ENABLED=0

DEB_BUILD_DIR=build/deb
DEB_PACKAGE_NAME=ssk_$(VERSION)_amd64.deb

MACOS_BUILD_DIR=build/macos
MACOS_PACKAGE_NAME_AMD64=ssk-$(VERSION)-amd64.pkg
MACOS_PACKAGE_NAME_ARM64=ssk-$(VERSION)-arm64.pkg
MACOS_PACKAGE_NAME_UNIVERSAL=ssk-$(VERSION)-universal.pkg
MACOS_IDENTIFIER=com.ssk.app

help: ## Display this help screen
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

build: ## Build the binary
	@echo "Building $(BINARY_NAME) v$(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=$(CGO_ENABLED) $(GOBUILD) -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME) v$(VERSION)"

run: build ## Build and run the application
	@echo "Running $(BINARY_NAME)..."
	@./$(BUILD_DIR)/$(BINARY_NAME)

test: ## Run tests
	@echo "Running tests..."
	$(GOTEST) -v ./...

test-short: ## Run tests without long-running tests
	@echo "Running short tests..."
	$(GOTEST) -v -short ./...

test-coverage: ## Run tests with coverage
	@echo "Running tests with coverage..."
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"
	@echo "Coverage summary:"
	@$(GOCMD) tool cover -func=coverage.out | tail -1

test-coverage-report: test-coverage ## Generate and open coverage report
	@echo "Opening coverage report in browser..."
	@xdg-open coverage.html 2>/dev/null || open coverage.html 2>/dev/null || echo "Please open coverage.html manually"

test-race: ## Run tests with race detector
	@echo "Running tests with race detector..."
	$(GOTEST) -race -v ./...

test-bench: ## Run benchmarks
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem ./...

install: ## Install the binary to $GOPATH/bin
	@echo "Installing $(BINARY_NAME)..."
	CGO_ENABLED=$(CGO_ENABLED) $(GOINSTALL) -ldflags="$(LDFLAGS)"
	@echo "Installation complete"

clean: ## Clean build artifacts
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)
	rm -rf $(DEB_BUILD_DIR)
	rm -rf $(MACOS_BUILD_DIR)
	rm -f coverage.out coverage.html
	@echo "Clean complete"

deps: ## Download dependencies
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy
	@echo "Dependencies downloaded"

lint: ## Run linter (check only)
	@echo "Running linter..."
	@test -x "$(GOLANGCI_LINT)" || (echo "golangci-lint not found. Run: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0" && exit 1)
	$(GOLANGCI_LINT) run ./...

lint-fix: ## Run linter and auto-fix issues
	@echo "Running linter with auto-fix..."
	@test -x "$(GOLANGCI_LINT)" || (echo "golangci-lint not found. Run: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0" && exit 1)
	$(GOLANGCI_LINT) run --fix ./...

fmt: ## Format code
	@echo "Formatting code..."
	$(GOCMD) fmt ./...
	@echo "Formatting complete"

vet: ## Run go vet
	@echo "Running go vet..."
	$(GOCMD) vet ./...

cross-compile: ## Build for multiple platforms
	@echo "Cross-compiling..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=$(CGO_ENABLED) $(GOBUILD) -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_PATH)
	GOOS=linux GOARCH=arm GOARM=6 CGO_ENABLED=$(CGO_ENABLED) $(GOBUILD) -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-armv6 $(MAIN_PATH)
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=$(CGO_ENABLED) $(GOBUILD) -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(MAIN_PATH)
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=$(CGO_ENABLED) $(GOBUILD) -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_PATH)
	@echo "Cross-compilation complete"

dev: ## Run in development mode with auto-reload (requires air)
	@which air > /dev/null || (echo "air not installed. Run: go install github.com/cosmtrek/air@latest" && exit 1)
	air

all: clean deps fmt vet test build ## Run all checks and build

build-deb: build ## Build Debian package
	@echo "Building Debian package v$(VERSION)..."
	@mkdir -p $(DEB_BUILD_DIR)/ssk_$(VERSION)_amd64/DEBIAN
	@mkdir -p $(DEB_BUILD_DIR)/ssk_$(VERSION)_amd64/usr/local/bin

	@cp $(BUILD_DIR)/$(BINARY_NAME) $(DEB_BUILD_DIR)/ssk_$(VERSION)_amd64/usr/local/bin/

	@echo "Package: ssk" > $(DEB_BUILD_DIR)/ssk_$(VERSION)_amd64/DEBIAN/control
	@echo "Version: $(VERSION)" >> $(DEB_BUILD_DIR)/ssk_$(VERSION)_amd64/DEBIAN/control
	@echo "Section: utils" >> $(DEB_BUILD_DIR)/ssk_$(VERSION)_amd64/DEBIAN/control
	@echo "Priority: optional" >> $(DEB_BUILD_DIR)/ssk_$(VERSION)_amd64/DEBIAN/control
	@echo "Architecture: amd64" >> $(DEB_BUILD_DIR)/ssk_$(VERSION)_amd64/DEBIAN/control
	@echo "Maintainer: Jon Doe <anonymous@ssk.com>" >> $(DEB_BUILD_DIR)/ssk_$(VERSION)_amd64/DEBIAN/control
	@echo "Description: Super Secret Kit - CLI tool for Jira, Tempo, and Arkevia" >> $(DEB_BUILD_DIR)/ssk_$(VERSION)_amd64/DEBIAN/control
	@echo " SSK provides secure credential management and integration" >> $(DEB_BUILD_DIR)/ssk_$(VERSION)_amd64/DEBIAN/control
	@echo " with Jira, Tempo, and Arkevia services." >> $(DEB_BUILD_DIR)/ssk_$(VERSION)_amd64/DEBIAN/control

	@cp scripts/postinst $(DEB_BUILD_DIR)/ssk_$(VERSION)_amd64/DEBIAN/
	@chmod 755 $(DEB_BUILD_DIR)/ssk_$(VERSION)_amd64/DEBIAN/postinst

	@dpkg-deb --build $(DEB_BUILD_DIR)/ssk_$(VERSION)_amd64
	@mv $(DEB_BUILD_DIR)/ssk_$(VERSION)_amd64.deb $(BUILD_DIR)/
	@echo "Debian package created: $(BUILD_DIR)/$(DEB_PACKAGE_NAME)"

build-deb-arm64: ## Build Debian package for ARM64
	@echo "Building Debian package for ARM64 v$(VERSION)..."
	@mkdir -p $(DEB_BUILD_DIR)/ssk_$(VERSION)_arm64/DEBIAN
	@mkdir -p $(DEB_BUILD_DIR)/ssk_$(VERSION)_arm64/usr/local/bin

	@GOOS=linux GOARCH=arm64 CGO_ENABLED=$(CGO_ENABLED) $(GOBUILD) -ldflags="$(LDFLAGS)" \
		-o $(DEB_BUILD_DIR)/ssk_$(VERSION)_arm64/usr/local/bin/$(BINARY_NAME) $(MAIN_PATH)

	@echo "Package: ssk" > $(DEB_BUILD_DIR)/ssk_$(VERSION)_arm64/DEBIAN/control
	@echo "Version: $(VERSION)" >> $(DEB_BUILD_DIR)/ssk_$(VERSION)_arm64/DEBIAN/control
	@echo "Section: utils" >> $(DEB_BUILD_DIR)/ssk_$(VERSION)_arm64/DEBIAN/control
	@echo "Priority: optional" >> $(DEB_BUILD_DIR)/ssk_$(VERSION)_arm64/DEBIAN/control
	@echo "Architecture: arm64" >> $(DEB_BUILD_DIR)/ssk_$(VERSION)_arm64/DEBIAN/control
	@echo "Maintainer: Jon Doe <anonymous@ssk.com>" >> $(DEB_BUILD_DIR)/ssk_$(VERSION)_arm64/DEBIAN/control
	@echo "Description: Super Secret Kit - CLI tool for Jira, Tempo, and Arkevia" >> $(DEB_BUILD_DIR)/ssk_$(VERSION)_arm64/DEBIAN/control
	@echo " SSK provides secure credential management and integration" >> $(DEB_BUILD_DIR)/ssk_$(VERSION)_arm64/DEBIAN/control
	@echo " with Jira, Tempo, and Arkevia services." >> $(DEB_BUILD_DIR)/ssk_$(VERSION)_arm64/DEBIAN/control

	@cp scripts/postinst $(DEB_BUILD_DIR)/ssk_$(VERSION)_arm64/DEBIAN/
	@chmod 755 $(DEB_BUILD_DIR)/ssk_$(VERSION)_arm64/DEBIAN/postinst

	@dpkg-deb --build $(DEB_BUILD_DIR)/ssk_$(VERSION)_arm64
	@mv $(DEB_BUILD_DIR)/ssk_$(VERSION)_arm64.deb $(BUILD_DIR)/
	@echo "Debian package created: $(BUILD_DIR)/ssk_$(VERSION)_arm64.deb"

build-deb-armv6: ## Build Debian armhf package for ARMv6 (Raspberry Pi Zero)
	@echo "Building Debian package for ARMv6/armhf v$(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	@mkdir -p $(DEB_BUILD_DIR)/ssk_$(VERSION)_armhf/DEBIAN
	@mkdir -p $(DEB_BUILD_DIR)/ssk_$(VERSION)_armhf/usr/local/bin

	@GOOS=linux GOARCH=arm GOARM=6 CGO_ENABLED=$(CGO_ENABLED) $(GOBUILD) -ldflags="$(LDFLAGS)" \
		-o $(DEB_BUILD_DIR)/ssk_$(VERSION)_armhf/usr/local/bin/$(BINARY_NAME) $(MAIN_PATH)

	@echo "Package: ssk" > $(DEB_BUILD_DIR)/ssk_$(VERSION)_armhf/DEBIAN/control
	@echo "Version: $(VERSION)" >> $(DEB_BUILD_DIR)/ssk_$(VERSION)_armhf/DEBIAN/control
	@echo "Section: utils" >> $(DEB_BUILD_DIR)/ssk_$(VERSION)_armhf/DEBIAN/control
	@echo "Priority: optional" >> $(DEB_BUILD_DIR)/ssk_$(VERSION)_armhf/DEBIAN/control
	@echo "Architecture: armhf" >> $(DEB_BUILD_DIR)/ssk_$(VERSION)_armhf/DEBIAN/control
	@echo "Maintainer: Jon Doe <anonymous@ssk.com>" >> $(DEB_BUILD_DIR)/ssk_$(VERSION)_armhf/DEBIAN/control
	@echo "Description: Super Secret Kit - CLI tool for Jira, Tempo, and Arkevia" >> $(DEB_BUILD_DIR)/ssk_$(VERSION)_armhf/DEBIAN/control
	@echo " SSK provides secure credential management and integration" >> $(DEB_BUILD_DIR)/ssk_$(VERSION)_armhf/DEBIAN/control
	@echo " with Jira, Tempo, and Arkevia services." >> $(DEB_BUILD_DIR)/ssk_$(VERSION)_armhf/DEBIAN/control

	@cp scripts/postinst $(DEB_BUILD_DIR)/ssk_$(VERSION)_armhf/DEBIAN/
	@chmod 755 $(DEB_BUILD_DIR)/ssk_$(VERSION)_armhf/DEBIAN/postinst

	@dpkg-deb --root-owner-group --build $(DEB_BUILD_DIR)/ssk_$(VERSION)_armhf
	@mv $(DEB_BUILD_DIR)/ssk_$(VERSION)_armhf.deb $(BUILD_DIR)/
	@echo "Debian package created: $(BUILD_DIR)/ssk_$(VERSION)_armhf.deb"

clean-deb: ## Clean Debian build artifacts
	@echo "Cleaning Debian build artifacts..."
	@rm -rf $(DEB_BUILD_DIR)
	@rm -f $(BUILD_DIR)/*.deb
	@echo "Clean complete"

build-pkg: ## Build macOS package for Intel (amd64)
	@echo "Building macOS package v$(VERSION) for Intel (amd64)..."
	@mkdir -p $(BUILD_DIR)
	@mkdir -p $(MACOS_BUILD_DIR)/ssk_$(VERSION)_amd64/payload/usr/local/bin
	@mkdir -p $(MACOS_BUILD_DIR)/ssk_$(VERSION)_amd64/scripts

	@GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) -ldflags="$(LDFLAGS)" \
		-o $(MACOS_BUILD_DIR)/ssk_$(VERSION)_amd64/payload/usr/local/bin/$(BINARY_NAME) $(MAIN_PATH)

	@cp scripts/postinstall.macos $(MACOS_BUILD_DIR)/ssk_$(VERSION)_amd64/scripts/postinstall
	@chmod 755 $(MACOS_BUILD_DIR)/ssk_$(VERSION)_amd64/scripts/postinstall

	@pkgbuild --root $(MACOS_BUILD_DIR)/ssk_$(VERSION)_amd64/payload \
		--scripts $(MACOS_BUILD_DIR)/ssk_$(VERSION)_amd64/scripts \
		--identifier $(MACOS_IDENTIFIER) \
		--version $(VERSION) \
		--install-location / \
		$(MACOS_BUILD_DIR)/ssk_$(VERSION)_amd64/ssk.pkg

	@productbuild --package $(MACOS_BUILD_DIR)/ssk_$(VERSION)_amd64/ssk.pkg \
		--identifier $(MACOS_IDENTIFIER) \
		--version $(VERSION) \
		$(BUILD_DIR)/$(MACOS_PACKAGE_NAME_AMD64)

	@echo "macOS package created: $(BUILD_DIR)/$(MACOS_PACKAGE_NAME_AMD64)"

build-pkg-arm64: ## Build macOS package for Apple Silicon (arm64)
	@echo "Building macOS package v$(VERSION) for Apple Silicon (arm64)..."
	@mkdir -p $(BUILD_DIR)
	@mkdir -p $(MACOS_BUILD_DIR)/ssk_$(VERSION)_arm64/payload/usr/local/bin
	@mkdir -p $(MACOS_BUILD_DIR)/ssk_$(VERSION)_arm64/scripts

	@GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 $(GOBUILD) -ldflags="$(LDFLAGS)" \
		-o $(MACOS_BUILD_DIR)/ssk_$(VERSION)_arm64/payload/usr/local/bin/$(BINARY_NAME) $(MAIN_PATH)

	@cp scripts/postinstall.macos $(MACOS_BUILD_DIR)/ssk_$(VERSION)_arm64/scripts/postinstall
	@chmod 755 $(MACOS_BUILD_DIR)/ssk_$(VERSION)_arm64/scripts/postinstall

	@pkgbuild --root $(MACOS_BUILD_DIR)/ssk_$(VERSION)_arm64/payload \
		--scripts $(MACOS_BUILD_DIR)/ssk_$(VERSION)_arm64/scripts \
		--identifier $(MACOS_IDENTIFIER) \
		--version $(VERSION) \
		--install-location / \
		$(MACOS_BUILD_DIR)/ssk_$(VERSION)_arm64/ssk.pkg

	@productbuild --package $(MACOS_BUILD_DIR)/ssk_$(VERSION)_arm64/ssk.pkg \
		--identifier $(MACOS_IDENTIFIER) \
		--version $(VERSION) \
		$(BUILD_DIR)/$(MACOS_PACKAGE_NAME_ARM64)

	@echo "macOS package created: $(BUILD_DIR)/$(MACOS_PACKAGE_NAME_ARM64)"

build-pkg-universal: ## Build universal macOS package (Intel + Apple Silicon)
	@echo "Building universal macOS package v$(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	@mkdir -p $(MACOS_BUILD_DIR)/ssk_$(VERSION)_universal/payload/usr/local/bin
	@mkdir -p $(MACOS_BUILD_DIR)/ssk_$(VERSION)_universal/scripts
	@mkdir -p $(MACOS_BUILD_DIR)/temp_binaries

	@GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 $(GOBUILD) -ldflags="$(LDFLAGS)" \
		-o $(MACOS_BUILD_DIR)/temp_binaries/$(BINARY_NAME)-amd64 $(MAIN_PATH)
	@GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 $(GOBUILD) -ldflags="$(LDFLAGS)" \
		-o $(MACOS_BUILD_DIR)/temp_binaries/$(BINARY_NAME)-arm64 $(MAIN_PATH)

	@lipo -create -output $(MACOS_BUILD_DIR)/ssk_$(VERSION)_universal/payload/usr/local/bin/$(BINARY_NAME) \
		$(MACOS_BUILD_DIR)/temp_binaries/$(BINARY_NAME)-amd64 \
		$(MACOS_BUILD_DIR)/temp_binaries/$(BINARY_NAME)-arm64

	@cp scripts/postinstall.macos $(MACOS_BUILD_DIR)/ssk_$(VERSION)_universal/scripts/postinstall
	@chmod 755 $(MACOS_BUILD_DIR)/ssk_$(VERSION)_universal/scripts/postinstall

	@pkgbuild --root $(MACOS_BUILD_DIR)/ssk_$(VERSION)_universal/payload \
		--scripts $(MACOS_BUILD_DIR)/ssk_$(VERSION)_universal/scripts \
		--identifier $(MACOS_IDENTIFIER) \
		--version $(VERSION) \
		--install-location / \
		$(MACOS_BUILD_DIR)/ssk_$(VERSION)_universal/ssk.pkg

	@productbuild --package $(MACOS_BUILD_DIR)/ssk_$(VERSION)_universal/ssk.pkg \
		--identifier $(MACOS_IDENTIFIER) \
		--version $(VERSION) \
		$(BUILD_DIR)/$(MACOS_PACKAGE_NAME_UNIVERSAL)

	@echo "Universal macOS package created: $(BUILD_DIR)/$(MACOS_PACKAGE_NAME_UNIVERSAL)"

clean-pkg: ## Clean macOS package build artifacts
	@echo "Cleaning macOS package artifacts..."
	@rm -rf $(MACOS_BUILD_DIR)
	@rm -f $(BUILD_DIR)/*.pkg
	@echo "Clean complete"

build-all-packages: build-deb build-pkg-universal ## Build both Debian and macOS packages
	@echo ""
	@echo "All packages built:"
	@ls -lh $(BUILD_DIR)/*.deb $(BUILD_DIR)/*.pkg 2>/dev/null || true
