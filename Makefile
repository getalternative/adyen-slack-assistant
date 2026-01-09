.PHONY: build clean deploy test

# Default stage (matches Doppler config names: stg, prod)
STAGE ?= stg

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

# Deploy to AWS (uses Doppler for secrets)
deploy: build
	doppler run --config $(STAGE) -- npx serverless deploy --stage $(STAGE)

# Deploy to production
deploy-prod: build
	doppler run --config prod -- npx serverless deploy --stage prod

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

# Local development with Doppler
run-local:
	doppler run --config $(STAGE) -- go run ./cmd/processor

# Show logs
logs-webhook:
	npx serverless logs -f webhook --stage $(STAGE) -t

logs-processor:
	npx serverless logs -f processor --stage $(STAGE) -t

# Setup Doppler (run once)
doppler-setup:
	doppler setup

# Show Doppler secrets (for debugging)
doppler-secrets:
	doppler secrets --config $(STAGE)
