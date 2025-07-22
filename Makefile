.PHONY: build clean test docker-build docker-up docker-down help

BUILDER := extend-builder

# Default target - show help
help:
	@echo "🚀 Extend Match Result Service - Available Commands"
	@echo "=================================================="
	@echo ""
	@echo "📦 Building & Development:"
	@echo "  build                    Build both services"
	@echo "  clean                    Clean build artifacts"
	@echo "  deps                     Install dependencies"
	@echo "  proto                    Generate protobuf files"
	@echo "  fmt                      Format code"
	@echo "  imports                  Fix imports and format code"
	@echo "  imports-check            Check if imports are properly formatted"
	@echo "  lint                     Run linter"
	@echo ""
	@echo "🧪 Testing:"
	@echo "  test                     Run unit tests"
	@echo "  test-all-go              Run all Go integration tests"
	@echo ""
	@echo "🎯 Specific Test Commands:"
	@echo "  test-specific            Run specific test by pattern"
	@echo "    Usage: make test-specific TEST=<test_pattern>"
	@echo "    Example: make test-specific TEST=TestKafkaMessageFlowTest"
	@echo "    Note: Two-player tests use x-mock-subject header for user simulation"
	@echo ""
	@echo "📋 Available Test Functions:"
	@echo "  Kafka Tests:"
	@echo "    TestKafkaMessageFlowTest     - Kafka message flow test"
	@echo "    TestKafkaMessageFormatTest   - Kafka message format test"
	@echo "    TestKafkaPerformanceTest     - Kafka performance test"
	@echo "    TestKafkaErrorHandlingTest   - Kafka error handling test"
	@echo "  Integration Tests:"
	@echo "    TestFullIntegrationTest      - Full integration test"
	@echo "    TestKafkaIntegrationTest     - Kafka integration test"
	@echo "    TestSenderValidationTest     - Sender validation test"
	@echo "  Two-Player Match Tests:"
	@echo "    TestTwoPlayerMatchComparisonTest - Two players submit matching results"
	@echo "    TestTwoPlayerMatchMismatchTest   - Two players submit conflicting results"
	@echo "    TestDuplicateSubmissionTest      - Duplicate submission handling test"
	@echo ""
	@echo "🐳 Docker Operations:"
	@echo "  docker-build             Build Docker images"
	@echo "  docker-build-force       Build Docker images (force rebuild)"
	@echo "  docker-clean             Clean Docker images"
	@echo "  docker-up                Start all services (with dependencies)"
	@echo "  docker-up-build          Start all services with rebuild"
	@echo "  docker-down              Stop all services"
	@echo ""
	@echo "📋 Service Logs:"
	@echo "  docker-logs              View all service logs (with follow)"
	@echo "  docker-logs-all          View all service logs (without follow)"
	@echo "  docker-logs-service      View specific service logs (with follow)"
	@echo "  docker-logs-service-all  View specific service logs (without follow)"
	@echo "    Usage: make docker-logs-service SERVICE=<service_name>"
	@echo "    Available services: collector, reconciliator"
	@echo ""
	@echo "🔧 Dependencies:"
	@echo "  dev-up                   Start dependencies (Kafka, Redis, Kafka UI)"
	@echo "  dev-up-build             Start dependencies with rebuild"
	@echo "  dev-down                 Stop dependencies"
	@echo "  dev-status               Check dependencies status"
	@echo ""

	@echo ""
	@echo "🏃‍♂️ Running Services:"
	@echo "  run-collector            Run Collector service locally"
	@echo "  run-collector-kafka      Run Collector service with Kafka"
	@echo "  run-reconciliator        Run Reconciliator service"
	@echo "  run-reconciliator-local  Run Reconciliator service (without env file)"
	@echo ""
	@echo "🐛 Debugging:"
	@echo "  install-delve            Install delve debugger"
	@echo "  debug-collector          Debug collector service with delve"
	@echo "  debug-reconciliator      Debug reconciliator service with delve"
	@echo ""
	@echo "📊 Monitoring & Data:"
	@echo "  kafka-topics             List Kafka topics"
	@echo "  kafka-consume            Consume messages from Kafka"
	@echo "  redis-cli                Connect to Redis CLI"
	@echo "  redis-keys               List Redis keys"
	@echo "  redis-matches            List match keys in Redis"
	@echo "  redis-get-match          Get specific match from Redis"
	@echo "    Usage: make redis-get-match MATCH_ID=<match_id>"
	@echo "    Note: Commands work for both dev and production environments"
	@echo ""
	@echo "⚙️  Configuration:"
	@echo "  env-check                Check environment configuration"
	@echo ""

	@echo ""
	@echo "💡 Examples:"
	@echo "  make help                Show this help message"
	@echo "  make docker-logs-service SERVICE=collector"
	@echo "  make dev-logs SERVICE=kafka"
	@echo "  make redis-get-match MATCH_ID=abc123"
	@echo "  make test-specific TEST=TestKafkaMessageFlowTest"
	@echo "  make test-specific TEST=TestFullIntegrationTest"
	@echo ""
	@echo "For more details, see README.md and deployments/docker/README.md"

# Build both services
build:
	@echo "Building services..."
	@mkdir -p bin
	@go build -o bin/collector ./cmd/collector
	@go build -o bin/reconciliator ./cmd/reconciliator
	@echo "Build completed!"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf bin/
	@go clean
	@echo "Clean completed!"

# Run tests
test:
	@echo "Running tests..."
	@go test -count=1 ./pkg/...

# Run linter
lint:
	@echo "Running linter..."
	@golangci-lint run

# Build Docker images
docker-build:
	@echo "Building Docker images..."
	@chmod +x scripts/docker-build.sh
	@./scripts/docker-build.sh

# Build Docker images (force rebuild)
docker-build-force:
	@echo "Building Docker images (force rebuild)..."
	@docker-compose -f deployments/docker/docker-compose.yml build --no-cache

# Clean Docker images
docker-clean:
	@echo "Cleaning Docker images..."
	@docker-compose -f deployments/docker/docker-compose.yml down
	@docker system prune -f
	@echo "Docker images cleaned!"

# Start services with Docker Compose (Docker)
docker-up:
	@echo "Starting all services with dependencies..."
	@if [ ! -f .env ]; then \
		echo "⚠️  .env file not found. Creating from .env.template..."; \
		cp .env.template .env; \
		echo "✅ .env file created. Please update it with your configuration."; \
	fi
	@echo "📄 Loading environment variables from .env file..."
	@bash -c 'set -a; source .env; set +a; docker-compose -f deployments/docker/docker-compose.yml up -d'

# Start services with Docker Compose (Docker) - Force rebuild
docker-up-build:
	@echo "Starting all services with rebuild..."
	@if [ ! -f .env ]; then \
		echo "⚠️  .env file not found. Creating from .env.template..."; \
		cp .env.template .env; \
		echo "✅ .env file created. Please update it with your configuration."; \
	fi
	@echo "📄 Loading environment variables from .env file..."
	@bash -c 'set -a; source .env; set +a; docker-compose -f deployments/docker/docker-compose.yml up -d --build'

# Stop services with Docker Compose (Docker)
docker-down:
	@echo "Stopping all services..."
	@docker-compose -f deployments/docker/docker-compose.yml down

# View logs for all services
docker-logs:
	@echo "Showing all service logs..."
	@docker-compose -f deployments/docker/docker-compose.yml logs -f

# View logs for specific service
docker-logs-service:
	@echo "Usage: make docker-logs-service SERVICE=<service_name>"
	@if [ -z "$(SERVICE)" ]; then \
		echo "❌ SERVICE is required"; \
		echo "Available services: collector, reconciliator"; \
		echo "Example: make docker-logs-service SERVICE=collector"; \
	else \
		echo "Showing logs for $(SERVICE)..."; \
		docker-compose -f deployments/docker/docker-compose.yml logs -f $(SERVICE); \
	fi

# View logs for all services (without follow)
docker-logs-all:
	@echo "Showing all service logs..."
	@docker-compose -f deployments/docker/docker-compose.yml logs

# View logs for specific service (without follow)
docker-logs-service-all:
	@echo "Usage: make docker-logs-service-all SERVICE=<service_name>"
	@if [ -z "$(SERVICE)" ]; then \
		echo "❌ SERVICE is required"; \
		echo "Available services: collector, reconciliator"; \
		echo "Example: make docker-logs-service-all SERVICE=collector"; \
	else \
		echo "Showing logs for $(SERVICE)..."; \
		docker-compose -f deployments/docker/docker-compose.yml logs $(SERVICE); \
	fi

# Run Collector service locally
run-collector:
	@echo "Starting Collector service..."
	@go run ./cmd/collector

# Run Reconciliator service locally (without env file)
run-reconciliator-local:
	@echo "Starting Reconciliator service..."
	@go run ./cmd/reconciliator

# Generate protobuf files
proto:
	@echo "Generating protobuf files..."
	@chmod +x scripts/proto.sh
	@./scripts/proto.sh
	@echo "Protobuf generation completed!"

# Install dependencies
deps:
	@echo "Installing dependencies..."
	@go mod download
	@go mod tidy

# Install delve debugger
install-delve:
	@echo "Installing delve debugger..."
	@chmod +x scripts/install-delve.sh
	@./scripts/install-delve.sh

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...

# Fix imports and format code
imports:
	@echo "Fixing imports and formatting..."
	@find . -name "*.go" -exec goimports -w {} \;

# Check if imports are properly formatted
imports-check:
	@echo "Checking imports formatting..."
	@if [ -n "$$(find . -name '*.go' -exec goimports -l {} \;)" ]; then \
		echo "The following files have import issues:"; \
		find . -name '*.go' -exec goimports -l {} \;; \
		exit 1; \
	else \
		echo "All imports are properly formatted!"; \
	fi

# Start dependencies (Kafka, Redis, Kafka UI)
dev-up:
	@echo "Starting dependencies (Kafka, Redis, Kafka UI)..."
	@if [ ! -f .env ]; then \
		echo "⚠️  .env file not found. Creating from .env.template..."; \
		cp .env.template .env; \
		echo "✅ .env file created. Please update it with your configuration."; \
	fi
	@echo "📄 Loading environment variables from .env file..."
	@bash -c 'set -a; source .env; set +a; docker-compose -f deployments/docker/docker-compose.dev.yml up -d'
	@echo "Waiting for Kafka to be ready..."
	@sleep 10
	@echo "Development environment started!"
	@echo "- Kafka: localhost:9092"
	@echo "- Kafka UI: http://localhost:9091"
	@echo "- Redis: localhost:6379"

# Start dependencies with rebuild
dev-up-build:
	@echo "Starting dependencies with rebuild..."
	@if [ ! -f .env ]; then \
		echo "⚠️  .env file not found. Creating from .env.template..."; \
		cp .env.template .env; \
		echo "✅ .env file created. Please update it with your configuration."; \
	fi
	@echo "📄 Loading environment variables from .env file..."
	@bash -c 'set -a; source .env; set +a; docker-compose -f deployments/docker/docker-compose.dev.yml up -d --build'
	@echo "Waiting for Kafka to be ready..."
	@sleep 10
	@echo "Development environment started!"
	@echo "- Kafka: localhost:9092"
	@echo "- Kafka UI: http://localhost:9091"
	@echo "- Redis: localhost:6379"

dev-down:
	@echo "Stopping dependencies..."
	@docker-compose -f deployments/docker/docker-compose.dev.yml down
	@echo "Dependencies stopped!"

dev-logs:
	@echo "Showing dependency logs..."
	@docker-compose -f deployments/docker/docker-compose.dev.yml logs -f kafka

# View logs for all dependencies
dev-logs-all:
	@echo "Showing all dependency logs..."
	@docker-compose -f deployments/docker/docker-compose.dev.yml logs -f

# View logs for specific dependency
dev-logs-service:
	@echo "Usage: make dev-logs-service SERVICE=<service_name>"
	@if [ -z "$(SERVICE)" ]; then \
		echo "❌ SERVICE is required"; \
		echo "Available services: kafka, redis, zipkin"; \
		echo "Example: make dev-logs-service SERVICE=kafka"; \
	else \
		echo "Showing logs for $(SERVICE)..."; \
		docker-compose -f deployments/docker/docker-compose.dev.yml logs -f $(SERVICE); \
	fi

# View logs for all dependencies (without follow)
dev-logs-all-static:
	@echo "Showing all dependency logs..."
	@docker-compose -f deployments/docker/docker-compose.dev.yml logs

# View logs for specific dependency (without follow)
dev-logs-service-static:
	@echo "Usage: make dev-logs-service-static SERVICE=<service_name>"
	@if [ -z "$(SERVICE)" ]; then \
		echo "❌ SERVICE is required"; \
		echo "Available services: kafka, redis, zipkin"; \
		echo "Example: make dev-logs-service-static SERVICE=kafka"; \
	else \
		echo "Showing logs for $(SERVICE)..."; \
		docker-compose -f deployments/docker/docker-compose.dev.yml logs $(SERVICE); \
	fi

dev-status:
	@echo "Checking dependencies status..."
	@docker-compose -f deployments/docker/docker-compose.dev.yml ps

# Run collector with Kafka configuration
run-collector-kafka:
	@echo "Starting Collector service with Kafka..."
	@if [ ! -f .env ]; then \
		echo "⚠️  .env file not found. Creating from .env.template..."; \
		cp .env.template .env; \
		echo "✅ .env file created. Please update it with your configuration."; \
	fi
	@echo "📄 Loading environment variables from .env file..."
	@bash -c 'set -a; source .env; set +a; go run ./cmd/collector'

# Run reconciliator with Kafka and Redis configuration
run-reconciliator:
	@echo "Starting Reconciliator service..."
	@if [ ! -f .env ]; then \
		echo "⚠️  .env file not found. Creating from .env.template..."; \
		cp .env.template .env; \
		echo "✅ .env file created. Please update it with your configuration."; \
	fi
	@echo "📄 Loading environment variables from .env file..."
	@bash -c 'set -a; source .env; set +a; go run ./cmd/reconciliator'

# Check environment configuration
env-check:
	@echo "🔍 Checking environment configuration..."
	@if [ ! -f .env ]; then \
		echo "❌ .env file not found"; \
		echo "📝 Run 'cp .env.template .env' to create from template"; \
	else \
		echo "✅ .env file found"; \
		echo ""; \
		echo "📋 Current configuration:"; \
		echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"; \
		cat .env | grep -v '^#' | grep -v '^$$' | sed 's/^/  /'; \
		echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"; \
		echo ""; \
		echo "🔧 To modify: edit .env file"; \
	fi

# Test all Go integration tests
test-all-go:
	@echo "Running all Go integration tests..."
	@if [ ! -f .env ]; then \
		echo "⚠️  .env file not found. Creating from .env.template..."; \
		cp .env.template .env; \
		echo "✅ .env file created. Please update it with your configuration."; \
	fi
	@echo "📄 Loading environment variables from .env file..."
	@bash -c 'set -a; source .env; set +a; go test -count=1 -v ./cmd/test/'

# Debug collector service with delve
debug-collector:
	@echo "Starting collector service with debugging..."
	@chmod +x scripts/debug-collector.sh
	@./scripts/debug-collector.sh

# Debug reconciliator service with delve
debug-reconciliator:
	@echo "Starting reconciliator service with debugging..."
	@if [ ! -f .env ]; then \
		echo "⚠️  .env file not found. Creating from .env.template..."; \
		cp .env.template .env; \
		echo "✅ .env file created. Please update it with your configuration."; \
	fi
	@echo "📄 Loading environment variables from .env file..."
	@bash -c 'set -a; source .env; set +a; dlv debug ./cmd/reconciliator --headless --listen=:2346 --api-version=2 --accept-multiclient --continue'

# View Kafka topics
kafka-topics:
	@echo "Listing Kafka topics..."
	@docker exec kafka /opt/kafka/bin/kafka-topics.sh --list --bootstrap-server localhost:9092

# View Kafka messages
kafka-consume:
	@echo "Consuming messages from match-results topic..."
	@docker exec -it kafka /opt/kafka/bin/kafka-console-consumer.sh --topic match-results --bootstrap-server localhost:9092 --from-beginning



# Redis operations
redis-cli:
	@echo "Connecting to Redis CLI..."
	@docker exec -it redis redis-cli

# View Redis keys
redis-keys:
	@echo "Listing Redis keys..."
	@docker exec redis redis-cli keys "*"

# View match data in Redis
redis-matches:
	@echo "Listing match keys in Redis..."
	@docker exec redis redis-cli keys "*:match:*"

# Get specific match from Redis
redis-get-match:
	@echo "Usage: make redis-get-match MATCH_ID=<match_id>"
	@if [ -z "$(MATCH_ID)" ]; then \
		echo "❌ MATCH_ID is required"; \
		echo "Example: make redis-get-match MATCH_ID=abc123"; \
	else \
		bash -c ' \
			if [ -f .env ]; then \
				set -a; source .env; set +a; \
				NAMESPACE="$${AB_NAMESPACE:-accelbyte}"; \
				echo "📄 Loaded namespace: $$NAMESPACE"; \
			else \
				NAMESPACE="accelbyte"; \
				echo "⚠️  .env file not found. Using default namespace '\''accelbyte'\''"; \
			fi; \
			echo "Getting match data for: $$NAMESPACE:match:$$MATCH_ID"; \
			docker exec redis redis-cli get "$$NAMESPACE:match:$$MATCH_ID"; \
		' bash --norc --noprofile MATCH_ID="$(MATCH_ID)"; \
	fi

# Test specific test by pattern
test-specific:
	@echo "Usage: make test-specific TEST=<test_pattern>"
	@if [ -z "$(TEST)" ]; then \
		echo "❌ TEST is required"; \
		echo "Example: make test-specific TEST=TestKafkaMessageFlowTest"; \
		echo ""; \
		echo "Available test patterns:"; \
		echo "  TestKafkaMessageFlowTest"; \
		echo "  TestKafkaMessageFormatTest"; \
		echo "  TestKafkaPerformanceTest"; \
		echo "  TestKafkaErrorHandlingTest"; \
		echo "  TestFullIntegrationTest"; \
		echo "  TestKafkaIntegrationTest"; \
		echo "  TestSenderValidationTest"; \
		echo "  TestTwoPlayerMatchComparisonTest"; \
		echo "  TestTwoPlayerMatchMismatchTest"; \
		echo "  TestDuplicateSubmissionTest"; \
	else \
		echo "Running specific test: $(TEST)"; \
		if [ ! -f .env ]; then \
			echo "⚠️  .env file not found. Creating from .env.template..."; \
			cp .env.template .env; \
			echo "✅ .env file created. Please update it with your configuration."; \
		fi; \
		echo "📄 Loading environment variables from .env file..."; \
		bash -c 'set -a; source .env; set +a; go test -count=1 -v -run "$(TEST)" ./cmd/test/'; \
	fi

# Buil and push image
imagex_push:
	@test -n "$(IMAGE_TAG)" || (echo "IMAGE_TAG is not set (e.g. 'v0.1.0', 'latest')"; exit 1)
	@test -n "$(REPO_URL)" || (echo "REPO_URL is not set"; exit 1)
	@test -n "$(DOCKERFILE_NAME)" || (echo "DOCKERFILE_NAME (e.g. './deployments/docker/collector.Dockerfile', './deployments/docker/reconciliator.Dockerfile') is not set"; exit 1)
	docker buildx inspect $(BUILDER) || docker buildx create --name $(BUILDER) --use
	docker buildx build -t ${REPO_URL}:${IMAGE_TAG} --platform linux/amd64 --push . --file ${DOCKERFILE_NAME}