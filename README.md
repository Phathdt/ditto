# Ditto

A service that helps implement the **Event-Driven architecture** by capturing PostgreSQL database changes and publishing them to message brokers.

To maintain the consistency of data in the system, we use **transactional messaging** - publishing events in a single transaction with a domain model change.

The service allows you to subscribe to changes in the PostgreSQL database using its logical decoding capability and publish them to Redis or other message brokers.

## 🚀 Features

- **Flexible Publication Strategies**: Single publication for all tables or individual publications per table
- **Auto-sync Publications**: Automatically creates and manages PostgreSQL publications
- **Redis Publishing**: Events published to Redis with configurable topics
- **Table Mapping**: Custom topic names for different tables
- **Real-time Processing**: Low-latency event processing using PostgreSQL WAL
- **Configuration-driven**: YAML-based configuration for easy management

## 📋 Table of Contents

- [Logic of Work](#logic-of-work)
- [Event Publishing](#event-publishing)
- [Configuration](#configuration)
- [Publication Strategies](#publication-strategies)
- [Database Setup](#database-setup)
- [Environment Variables](#environment-variables)
- [Usage](#usage)
- [Tools & Scripts](#tools--scripts)
- [Documentation](#documentation)
- [Docker & CI/CD](#docker--cicd)

## Logic of Work

To receive events about data changes in our PostgreSQL DB, we use the standard logical decoding module (**pgoutput**). This module converts changes read from the WAL into a logical replication protocol, and we consume all this information on our side.

Then we filter out only the events we need and publish them to Redis with configurable topics.

## Event Publishing

Service currently supports **Redis** as the message broker.

The service publishes the following structure to Redis topics:

```go
{
	ID        uuid.UUID       // unique ID
	Schema    string
	Table     string
	Action    string          // insert, update, delete
	Data      map[string]any  // new data
	DataOld   map[string]any  // old data (for updates/deletes)
	EventTime time.Time       // commit time
}
```

**Topic Structure**: `{prefix_watch_list}.{mapping}`

Messages are published to the broker **at least once**!

## Configuration

Configuration is managed via `config/config.yml` file:

```yaml
# Publication strategy: "single" (default) or "multiple"
publication_strategy: "single"
publication_prefix: "ditto"        # used for multiple strategy

# Redis topic prefix
prefix_watch_list: "events"

# Tables to watch
watch_list:
  deposit_events:
    mapping: "deposits"    # custom topic name (optional)
  withdraw_events:
    mapping: "withdrawals"
  loan_events:
    mapping: "loans"
```

### Configuration Options

| Field | Description | Default |
|-------|-------------|---------|
| `publication_strategy` | "single" or "multiple" | "single" |
| `publication_prefix` | Prefix for multiple publications | "ditto" |
| `prefix_watch_list` | Redis topic prefix | "" |
| `watch_list` | Tables to monitor | {} |
| `mapping` | Custom topic name for table | table name |

## 📊 Publication Strategies

Ditto supports two publication strategies:

### 1. Single Publication (Recommended)
- **One publication** for all tables
- **Simple and efficient**
- **Lower resource usage**
- **Easy to maintain**

```yaml
publication_strategy: "single"  # or omit (default)
```

**Results in**: `ditto` publication with all specified tables

### 2. Multiple Publications
- **Individual publication** per table
- **Better fault isolation**
- **More flexible scaling**
- **Higher resource usage**

```yaml
publication_strategy: "multiple"
publication_prefix: "ditto"
```

**Results in**: `ditto_deposit_events`, `ditto_withdraw_events`, etc.

👉 **See [Publication Strategies Guide](docs/PUBLICATION_STRATEGIES.md) for detailed comparison**

## Database Setup

### PostgreSQL Configuration

You must make the following settings in `postgresql.conf`:

```ini
wal_level = logical
max_replication_slots >= 1
max_wal_senders >= 1
```

### Replica Identity (Optional)

To receive `DataOld` field for UPDATE/DELETE operations:

```sql
ALTER TABLE your_table REPLICA IDENTITY FULL;
```

### Manual Publication Management

Publications are **automatically created and managed** by the service. However, you can also manage them manually:

```sql
-- Check current publications
SELECT * FROM pg_publication;

-- Create custom publication
CREATE PUBLICATION ditto FOR TABLE table1, table2;

-- Drop publication
DROP PUBLICATION IF EXISTS ditto;
```

## Environment Variables

```bash
# Database connection (with replication=database)
DB_DSN="postgresql://postgres:password@localhost:5432/dbname?replication=database"

# Redis connection
REDIS_URL="redis://localhost:6379"

# Optional: Log level
LOG_LEVEL="info"

# Optional: Application environment
APP_ENV="dev"
```

## 🔧 Usage

### Quick Start with Docker Compose

```bash
# 1. Copy example files
cp config/config.example.yml config/config.yml
cp docker-compose.example.yml docker-compose.yml
cp init-db.example.sql init-db.sql

# 2. Edit configuration if needed
nano config/config.yml

# 3. Start all services
docker-compose up -d

# 4. Watch logs
docker-compose logs -f ditto

# 5. Test with sample data
docker-compose exec postgres psql -U postgres -d ditto_db -c "SELECT generate_test_events(10);"

# 6. Monitor Redis events (optional)
docker-compose --profile debug up redis-cli
```

### Manual Setup

#### 1. Setup Configuration

```bash
# Copy example config
cp config/config.example.yml config/config.yml

# Edit your configuration
nano config/config.yml
```

#### 2. Set Environment Variables

```bash
export DB_DSN="postgresql://postgres:password@localhost:5432/dbname?replication=database"
export REDIS_URL="redis://localhost:6379"
```

#### 3. Run the Service

```bash
# Using Go
go run main.go

# Using Docker
docker build -t ditto .
docker run --env-file .env ditto

# Using Task
task run
```

## 🛠 Tools & Scripts

### Publication Check Script

Use the SQL script to verify publications:

```bash
# Check current publication status
psql -d your_database -f scripts/check_publications.sql
```

### Release Script

Automated release script that creates tags and triggers CI/CD:

```bash
# Create a new release
./scripts/release.sh v1.0.0

# This will:
# - Validate version format
# - Check git status
# - Create and push git tag
# - Trigger GitHub Actions to build Docker image and create release
```

### Example Output Topics

With configuration:
```yaml
prefix_watch_list: "events"
watch_list:
  deposit_events:
    mapping: "deposits"
  withdraw_events:
    mapping: "withdrawals"
```

**Published topics**:
- `events.deposits`
- `events.withdrawals`

## 📚 Documentation

- [Publication Strategies Guide](docs/PUBLICATION_STRATEGIES.md) - Detailed comparison of strategies
- [Configuration Examples](config/config.example.yml) - Sample configurations

## 🏗 Architecture

```
PostgreSQL WAL → Logical Decoding → Ditto Service → Redis Topics
     ↓              ↓                    ↓             ↓
   Tables    →   pgoutput    →    Event Processing → Consumer Apps
```

## 🐳 Docker & CI/CD

### Docker Images

Pre-built Docker images are available on Docker Hub:

```bash
# Latest version
docker pull phathdt379/ditto:latest

# Specific version
docker pull phathdt379/ditto:v1.0.0

# Run with environment variables
docker run --env-file .env phathdt379/ditto:latest
```

### Automated Releases

Releases are automated via GitHub Actions:

1. **Create Release**: Push a git tag (e.g., `v1.0.0`)
2. **Auto Build**: GitHub Actions builds multi-platform Docker images
3. **Auto Deploy**: Images pushed to Docker Hub
4. **GitHub Release**: Automatically created with release notes

### Docker Compose Example

```yaml
version: '3.8'
services:
  ditto:
    image: phathdt379/ditto:latest
    environment:
      - DB_DSN=postgresql://postgres:password@postgres:5432/dbname?replication=database
      - REDIS_URL=redis://redis:6379
      - LOG_LEVEL=info
    volumes:
      - ./config:/app/config
    depends_on:
      - postgres
      - redis

  postgres:
    image: postgres:15
    environment:
      POSTGRES_DB: dbname
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: password
    command: >
      postgres
      -c wal_level=logical
      -c max_replication_slots=4
      -c max_wal_senders=4

  redis:
    image: redis:7-alpine
```

## 🚧 TODO

- [ ] Support multiple message brokers (NATS, Kafka)
- [ ] Add condition-based filtering
- [ ] Web UI for configuration management
- [ ] Metrics and monitoring
- [ ] Cluster support
- [ ] Dead letter queue handling
- [ ] Schema evolution support

## 🤝 Contributing

1. Fork the repository
2. Create your feature branch
3. Make your changes
4. Add tests if applicable
5. Submit a pull request

## 📄 License

This project is licensed under the MIT License.

---

**Note**: This service is designed for high-throughput, low-latency event processing. Make sure your PostgreSQL and Redis instances are properly configured for your expected load.
