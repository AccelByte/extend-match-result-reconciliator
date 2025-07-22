# Match Result Collector Service

A gRPC service that collects match results and publishes them to Kafka for further processing.

## Features

- **gRPC API**: Secure match result submission with protobuf definitions
- **HTTP Gateway**: RESTful API with Swagger documentation
- **Kafka Integration**: Publishes match results to Kafka topics
- **Metrics**: Prometheus metrics endpoint
- **Health Check**: Built-in health check endpoint
- **Logging**: Structured logging with logrus
- **Tracing**: OpenTelemetry distributed tracing

## Documentation

For more detailed documentation, see the [docs](./docs/README.md) directory.

## API Endpoints

### gRPC
- **Service**: `Service.SubmitMatchResult`
- **Port**: 6565

### HTTP
- **POST** `/v1/matches/{match_id}/results` - Submit match result
- **GET** `/health` - Health check
- **GET** `/metrics` - Prometheus metrics  
- **GET** `/collector/apidocs/` - Swagger UI

## Match Result Format

```json
{
  "players": ["userA", "userB"],
  "stats": {
    "winner": "userA",
    "playerStats": {
      "userA": { "mmr": 10 },
      "userB": { "mmr": -5 }
    }
  },
  "startTime": 1723871234,
  "endTime": 1723871234
}
```

## Kafka Message Format

The service publishes messages to Kafka in this format:

```json
{
  "matchID": "abc123",
  "players": ["userA", "userB"],
  "stats": {
    "winner": "userA",
    "userA": { "mmr": 10 },
    "userB": { "mmr": -5 }
  },
  "startTime": 1723871234,
  "endTime": 1723871234,
  "submittedAt": "2025-01-13T15:30:00Z",
  "version": 1
}
```

## Architecture

The system consists of two main services:

1. **Match Result Collector**: Receives match results via gRPC/HTTP API and publishes them to Kafka
2. **Reconciliator**: Consumes match results from Kafka, compares them, reconciles discrepancies with AccelByte Session Service, stores the final result in AccelByte CloudSave, and updates player statistics in AccelByte Statistics. Redis is used for temporary storage during comparison.

```
HTTP/gRPC API → Collector → Kafka → Reconciliator → Redis (temp) / AccelByte CloudSave / AccelByte Statistics
```

## Getting Started

### Quick Start (Development)

1. **Configure Environment Variables**
   ```bash
   # Copy the template configuration
   cp .env.template .env
   
   # Edit .env with your configuration
   nano .env
   ```

2. **Start Infrastructure**
   ```bash
   # Start Kafka and Redis using Docker Compose
   make dev-up
   ```

3. **Start Services**
   ```bash
   # Terminal 1: Start Collector
   make run-collector-kafka
   
   # Terminal 2: Start Reconciliator
   make run-reconciliator
   ```

4. **Test Integration**
   ```bash
   # Run all Go integration tests
   make test-all-go
   ```

### Manual Setup

1. **Build Services**
   ```bash
   # Build both services
   make build
   ```

2. **Run Services**
   ```bash
   # Run collector
   ./bin/collector
   
   # Run reconciliator
   ./bin/reconciliator
   ```

## Configuration

### Required Environment Variables

**Collector Service:**
- `AB_BASE_URL`: AccelByte instance URL
- `AB_NAMESPACE`: AccelByte namespace
- `AB_CLIENT_ID`: AccelByte client ID
- `AB_CLIENT_SECRET`: AccelByte client secret
- `KAFKA_BROKERS`: Kafka broker addresses (e.g., `localhost:9092`)

**Reconciliator Service:**
- `KAFKA_BROKERS`: Kafka broker addresses (e.g., `localhost:9092`)
- `REDIS_ADDR`: Redis address (default: `localhost:6379`)

### IAM Client Permissions

The following IAM permissions are required for the AccelByte client credentials:

**Reconciliator Service:**
- `NAMESPACE:{namespace}:SESSION:GAME [Read]` - Required for session validation when `AB_SESSION_VALIDATION_ENABLED=true`
- `ADMIN:NAMESPACE:{namespace}:USER:*:CLOUDSAVE:RECORD [Read, Update]` - Required for reading and updating player records in CloudSave
- `ADMIN:NAMESPACE:{namespace}:USER:*:STATITEM [Update]` - Required for updating player statistics

**Integration Tests:**
- `NAMESPACE:{namespace}:SESSION:GAME [Create, Read, Update, Delete]` - Required for creating and managing test game sessions
- `ADMIN:NAMESPACE:{namespace}:SESSION:CONFIGURATION [Create, Read, Update, Delete]` - Required for managing session configuration templates during testing
- `ADMIN:NAMESPACE:{namespace}:USER:*:CLOUDSAVE:RECORD [Create, Read, Delete]` - Required for creating and managing test player records in CloudSave
- `ADMIN:NAMESPACE:{namespace}:USER:*:STATITEM [Create, Read, Update, Delete]` - Required for creating and managing test player statistics
- `ADMIN:NAMESPACE:{namespace}:STAT [Create, Read, Delete]` - Required for creating and managing stat configurations during testing

> **Note**: Replace `{namespace}` with your actual AccelByte namespace. The Reconciliator Service only requires Read permissions for session validation and CloudSave operations, while integration tests need full CRUD permissions to create test sessions, manage configurations, and test CloudSave functionality.

### Optional Settings

**Kafka Configuration:**
- `KAFKA_TOPIC`: Kafka topic name (default: `{namespace}.match`)
- `KAFKA_GROUP_ID`: Consumer group ID (default: `reconciliator-group`)
- `KAFKA_MIN_BYTES`: Minimum bytes per fetch (default: `10240`)
- `KAFKA_MAX_BYTES`: Maximum bytes per fetch (default: `10485760`)

**Kafka Producer Batching (Collector Service):**
- **Default Behavior**: Messages are batched before sending to Kafka
- **Batch Size**: Up to 100 messages per batch
- **Batch Timeout**: 1 second maximum wait time
- **Effect**: Messages may be delayed by up to 1 second for batching efficiency
- **Configuration**: Can be customized via Kafka writer settings in code if needed

**Redis Configuration:**
- `REDIS_PASSWORD`: Redis password (if required)
- `REDIS_DB`: Redis database number (default: `0`)
- `REDIS_TTL_SECONDS`: Data TTL in seconds (default: `300` = 5 minutes)

**Service Configuration:**
- `LOG_LEVEL`: Log level (default: `info`)
- `LOG_FORMAT`: Log format - `json` or `text` (default: `json`)
- `SERVICE_NAME`: Service name for logging
- `SERVICE_VERSION`: Service version for logging
- `ENVIRONMENT`: Environment name (default: `development`)

### Service Behavior

**Collector Service:**
- Publishes all match results to Kafka when configured
- Uses match ID as the message key for partitioning
- Includes metadata headers (content-type, version)
- Continues to function without Kafka (with warning logs)
- **Kafka Batching**: Uses default Kafka writer settings that batch messages:
  - Waits for up to 100 messages OR 1 second before sending to Kafka
  - This balances throughput with latency for most use cases
  - Can be configured via `BatchSize` and `BatchTimeout` if different behavior is needed

**Reconciliator Service:**
- Consumes messages from Kafka in a consumer group
- Stores match results in Redis with key format `{namespace}:match:{matchID}`
- Validates message structure before storage
- Supports graceful shutdown and health checks

## Development

### Available Make Commands

**Services and dependencies:**
```bash
make docker-up          # Start services and dependencies
make docker-down        # Stop services and dependencies
```

**Logging:**
```bash
# Service logs
make docker-logs                     # View all service logs (with follow)
make docker-logs-all                 # View all service logs (without follow)
make docker-logs-service SERVICE=collector  # View specific service logs (with follow)
make docker-logs-service-all SERVICE=collector  # View specific service logs (without follow)
```

**Building:**
```bash
make build           # Build both services
make clean           # Clean build artifacts
make proto           # Generate protobuf files
```

**Running Services:**
```bash
make run-collector-kafka    # Run collector with Kafka
make run-reconciliator      # Run reconciliator with Redis
make run-collector          # Run collector without Kafka
```

**Testing:**
```bash
make test               # Run unit tests
make test-all-go        # Run all Go integration tests
make test-specific TEST=<pattern>  # Run specific test by pattern
```

**Kafka Operations:**
```bash
make kafka-topics       # List Kafka topics
make kafka-consume      # Consume messages from Kafka
```

**Redis Operations:**
```bash
make redis-cli          # Connect to Redis CLI
make redis-keys         # List all Redis keys
make redis-matches      # List match keys
make redis-get-match MATCH_ID=abc123  # Get specific match
```

**Utilities:**
```bash
make env-check          # Check environment configuration
make fmt               # Format code
make lint              # Run linter
```

## Monitoring

- **Metrics**: Available at `:8080/metrics`
- **Health Check**: Available at `:8000/health`
- **Logs**: Structured JSON logs via logrus

## Test Observability

To see how observability works in this project during local development, set up the following before testing:

1. Add or uncomment the loki logging driver in [deployments/docker/docker-compose.yml](deployments/docker/docker-compose.yml) for the collector and reconciliator services. Example configuration:

   ```
   logging:
     driver: loki
     options:
       loki-url: http://host.docker.internal:3100/loki/api/v1/push
       mode: non-blocking
       max-buffer-size: 4m
       loki-retries: "3"
   ```

   > :warning: **Make sure to install the docker loki plugin beforehand**: Otherwise, the app will not run. This is required so that container logs can flow to the `loki` service within the `grpc-plugin-dependencies` stack. Use this command to install the docker loki plugin: `docker plugin install grafana/loki-docker-driver:latest --alias loki --grant-all-permissions`.
   >
   > :info: **For Linux users**: `host.docker.internal` may not work natively. As a workaround, replace it with your Docker bridge IP (e.g., `172.17.0.1` - find it with `ip addr show docker0`). For a more seamless setup, see the shared network option below.

2. Clone and run the [grpc-plugin-dependencies](https://github.com/AccelByte/grpc-plugin-dependencies) stack alongside this app. After this, Grafana will be accessible at http://localhost:3000.

   ```
   git clone https://github.com/AccelByte/grpc-plugin-dependencies.git
   cd grpc-plugin-dependencies
   docker compose up
   ```

   > :exclamation: More information about [grpc-plugin-dependencies](https://github.com/AccelByte/grpc-plugin-dependencies) is available [here](https://github.com/AccelByte/grpc-plugin-dependencies/blob/main/README.md).

3. Perform testing. For example, by starting the services with `make docker-up` and running integration tests with `make test-all-go`, or sending manual requests as described in the docs.

### Zipkin Traces Setup
The setup for sending traces to Zipkin (via OpenTelemetry) is similar to Loki for logs. Traces are exported using the `OTEL_EXPORTER_ZIPKIN_ENDPOINT` environment variable, which defaults to `http://localhost:9411/api/v2/spans` in `.env.template`. For container access:

- Use `http://host.docker.internal:9411/api/v2/spans` (or your Docker bridge IP on Linux, e.g., `http://172.17.0.1:9411/api/v2/spans`).
- This points to the otel-collector in grpc-plugin-dependencies, which receives on port 9411 and forwards to Tempo.
- The variable is passed to services in `deployments/docker/docker-compose.yml`.

**Viewing Traces in Grafana**:
- In Grafana (http://localhost:3000), go to Explore > Select Tempo data source.
- Query traces, e.g., by service name like `{service="ExtendMatchResultCollector"}`.
- Or browse traces in the Tempo dashboard.

For Linux compatibility and seamless access, use the shared network setup below, updating the endpoint to `http://otel-collector:9411/api/v2/spans`.

### Advanced: Shared Network for Better Compatibility
To make logging more reliable across platforms (especially Linux), you can set up a shared Docker network between this project and grpc-plugin-dependencies:

1. Create a shared network: `docker network create observability-net`

2. In this project's `deployments/docker/docker-compose.yml`, add the shared network under `networks` at the bottom and reference it in services:
   ```yaml
   networks:
     app-network:
       driver: bridge
     observability-net:
       external: true
   ```
   Then, for each service (collector, reconciliator), add `networks: - app-network - observability-net`

3. In grpc-plugin-dependencies' `docker-compose.yaml`, add the shared network similarly (e.g., under services like loki: `networks: - observability-net`).

4. Update the `loki-url` in this project's docker-compose.yml to `http://loki:3100/loki/api/v1/push` (using the service name directly).

This allows direct container-to-container communication without relying on host IPs.

## Build and Push Image

- **Docker Login**: <br> Use [extend-helper-cli](https://docs.accelbyte.io/gaming-services/services/extend/extend-helper-cli/). Sample command:
  ```sh
  ./extend-helper-cli dockerlogin --namespace extendsystemtest-game --app extend-match-result --login
  ```
- **Build and Push**: <br>
  Sample command to build and push reconciliator image. Update variable `DOCKERFILE_NAME` to `./deployments/docker/collector.Dockerfile` for collector app.
  ```sh
  REPO_URL="044322590703.dkr.ecr.us-east-2.amazonaws.com/foundations/justice/stage/extend/ext-extendsystemtest-game-dtpu6/extend-match-result" \
  IMAGE_TAG=v0.0.1 \
  DOCKERFILE_NAME="./deployments/docker/reconciliator.Dockerfile" \
  make imagex_push
  ```
