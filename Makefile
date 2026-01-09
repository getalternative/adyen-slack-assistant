.PHONY: build clean deploy test

# Build all Lambda functions
build:
	@echo "Building webhook..."
	GOOS=linux GOARCH=arm64 go build -tags lambda.norpc -o bin/webhook/bootstrap ./cmd/webhook
	cd bin/webhook && zip ../webhook.zip bootstrap

	@echo "Building processor..."
	GOOS=linux GOARCH=arm64 go build -tags lambda.norpc -o bin/processor/bootstrap ./cmd/processor
	cd bin/processor && zip ../processor.zip bootstrap

# Clean build artifacts
clean:
	rm -rf bin/

# Deploy to AWS
deploy: build
	serverless deploy --stage $(STAGE)

# Deploy to production
deploy-prod: build
	serverless deploy --stage prod

# Run tests
test:
	go test -v ./...

# Download dependencies
deps:
	go mod download
	go mod tidy

# Format code
fmt:
	go fmt ./...

# Lint code
lint:
	golangci-lint run

# Local development (requires docker)
local:
	serverless offline

# Show logs
logs-webhook:
	serverless logs -f webhook --stage $(STAGE) -t

logs-processor:
	serverless logs -f processor --stage $(STAGE) -t

# Default stage
STAGE ?= dev
