.PHONY: build run clean test docker-build docker-run install

# Binary name
BINARY_NAME=mcp-memory-server
DOCKER_IMAGE=mcp-memory-server

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOINSTALL=$(GOCMD) install

# Build the binary
build:
	$(GOBUILD) -o $(BINARY_NAME) -v ./cmd/mcp-memory-server

# Run the server
run: build
	./$(BINARY_NAME)

# Install the binary to $GOPATH/bin
install:
	$(GOINSTALL) ./cmd/mcp-memory-server

# Clean build artifacts
clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f memory.db

# Run tests
test:
	$(GOTEST) -v ./...

# Build Docker image
docker-build:
	docker build -t $(DOCKER_IMAGE) .

# Run Docker container
docker-run:
	docker run -i -v mcp-memory-data:/data $(DOCKER_IMAGE)

# Build for multiple platforms
build-all:
	GOOS=darwin GOARCH=amd64 $(GOBUILD) -o $(BINARY_NAME)-darwin-amd64 ./cmd/mcp-memory-server
	GOOS=darwin GOARCH=arm64 $(GOBUILD) -o $(BINARY_NAME)-darwin-arm64 ./cmd/mcp-memory-server
	GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BINARY_NAME)-linux-amd64 ./cmd/mcp-memory-server
	GOOS=linux GOARCH=arm64 $(GOBUILD) -o $(BINARY_NAME)-linux-arm64 ./cmd/mcp-memory-server
	GOOS=windows GOARCH=amd64 $(GOBUILD) -o $(BINARY_NAME)-windows-amd64.exe ./cmd/mcp-memory-server

# Download dependencies
deps:
	$(GOGET) -u ./...
	$(GOCMD) mod tidy

# Format code
fmt:
	$(GOCMD) fmt ./...

# Lint code
lint:
	golangci-lint run

# Development mode with hot reload (requires air)
dev:
	air -c .air.toml