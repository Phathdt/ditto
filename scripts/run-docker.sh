#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}🚀 Ditto Docker Runner${NC}"
echo "=========================="

# Default values
IMAGE_NAME="ditto-test"
CONTAINER_NAME="ditto-container"
DETACH=false
FOLLOW_LOGS=false
REMOVE_EXISTING=false

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -i|--image)
            IMAGE_NAME="$2"
            shift 2
            ;;
        -n|--name)
            CONTAINER_NAME="$2"
            shift 2
            ;;
        -d|--detach)
            DETACH=true
            shift
            ;;
        -f|--follow-logs)
            FOLLOW_LOGS=true
            shift
            ;;
        -r|--remove-existing)
            REMOVE_EXISTING=true
            shift
            ;;
        -h|--help)
            echo "Usage: $0 [OPTIONS] [COMMAND]"
            echo ""
            echo "Options:"
            echo "  -i, --image IMAGE_NAME       Docker image to run (default: ditto-test)"
            echo "  -n, --name CONTAINER_NAME    Container name (default: ditto-container)"
            echo "  -d, --detach                 Run in detached mode"
            echo "  -f, --follow-logs            Follow logs after starting (only with -d)"
            echo "  -r, --remove-existing        Remove existing container with same name"
            echo "  -h, --help                   Show this help"
            echo ""
            echo "Examples:"
            echo "  $0                           # Run with default settings"
            echo "  $0 -d                        # Run in detached mode"
            echo "  $0 -d -f                     # Run detached and follow logs"
            echo "  $0 -i phathdt379/ditto:latest  # Use production image"
            echo "  $0 -r                        # Remove existing and run new"
            echo ""
            echo "Commands:"
            echo "  $0 stop                      # Stop the container"
            echo "  $0 restart                   # Restart the container"
            echo "  $0 logs                      # Show container logs"
            echo "  $0 exec                      # Execute shell in container"
            exit 0
            ;;
        stop)
            echo -e "${YELLOW}🛑 Stopping container: $CONTAINER_NAME${NC}"
            docker stop "$CONTAINER_NAME" 2>/dev/null || echo "Container not running"
            exit 0
            ;;
        restart)
            echo -e "${YELLOW}🔄 Restarting container: $CONTAINER_NAME${NC}"
            docker restart "$CONTAINER_NAME" 2>/dev/null || echo "Container not found"
            exit 0
            ;;
        logs)
            echo -e "${YELLOW}📋 Showing logs for: $CONTAINER_NAME${NC}"
            docker logs -f "$CONTAINER_NAME" 2>/dev/null || echo "Container not found"
            exit 0
            ;;
        exec)
            echo -e "${YELLOW}💻 Executing shell in: $CONTAINER_NAME${NC}"
            docker exec -it "$CONTAINER_NAME" /bin/sh 2>/dev/null || echo "Container not running"
            exit 0
            ;;
        *)
            echo -e "${RED}❌ Unknown option: $1${NC}"
            exit 1
            ;;
    esac
done

# Check if Docker is running
if ! docker info > /dev/null 2>&1; then
    echo -e "${RED}❌ Docker is not running${NC}"
    exit 1
fi

# Check if image exists
if ! docker image inspect "$IMAGE_NAME" > /dev/null 2>&1; then
    echo -e "${RED}❌ Docker image '$IMAGE_NAME' not found${NC}"
    echo "Available images:"
    docker images | grep -E "(REPOSITORY|ditto)"
    echo ""
    echo "Build the image first:"
    echo "  ./scripts/build-docker.sh --load -p linux/arm64"
    exit 1
fi

# Check for .env file and fix localhost issues
if [[ ! -f ".env" ]]; then
    echo -e "${YELLOW}⚠️  .env file not found, creating example...${NC}"
    cat > .env << EOF
# Database connection (with replication=database)
DB_DSN=postgresql://postgres:password@host.docker.internal:5432/ditto_db?replication=database

# Redis connection
REDIS_URL=redis://host.docker.internal:6379

# Optional: Log level
LOG_LEVEL=info

# Optional: Application environment
APP_ENV=dev
EOF
    echo -e "${GREEN}✅ Created .env file with examples${NC}"
else
    # Check if .env contains localhost and suggest fixes
    if grep -q "localhost" .env 2>/dev/null; then
        echo -e "${YELLOW}⚠️  Found 'localhost' in .env file${NC}"
        echo "In Docker containers, 'localhost' refers to the container itself, not the host machine."
        echo ""
        echo -e "${BLUE}Current .env contains:${NC}"
        grep "localhost" .env || true
        echo ""
        echo -e "${GREEN}Suggested fixes:${NC}"
        grep "localhost" .env | sed 's/localhost/host.docker.internal/g' || true
        echo ""
        read -p "Auto-fix localhost -> host.docker.internal? (y/N): " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            # Create backup
            cp .env .env.backup
            # Fix localhost
            sed -i.bak 's/localhost/host.docker.internal/g' .env
            echo -e "${GREEN}✅ Fixed .env file (backup saved as .env.backup)${NC}"
        else
            echo -e "${YELLOW}⚠️  Please manually update .env file before running${NC}"
            echo "The container may not be able to connect to host services."
        fi
    fi
fi

# Check for config directory
if [[ ! -d "config" ]]; then
    echo -e "${YELLOW}⚠️  config directory not found, creating example...${NC}"
    mkdir -p config
    cat > config/config.yml << EOF
# Publication strategy: "single" (default) or "multiple"
publication_strategy: "single"
publication_prefix: "ditto"

# Redis topic prefix
prefix_watch_list: "events"

# Tables to watch
watch_list:
  deposit_events:
    mapping: "deposits"
  withdraw_events:
    mapping: "withdrawals"
  loan_events:
    mapping: "loans"
EOF
    echo -e "${GREEN}✅ Created config/config.yml with examples${NC}"
fi

# Remove existing container if requested
if [[ "$REMOVE_EXISTING" == "true" ]]; then
    echo -e "${YELLOW}🗑️  Removing existing container: $CONTAINER_NAME${NC}"
    docker rm -f "$CONTAINER_NAME" 2>/dev/null || echo "No existing container to remove"
fi

# Check if container already exists
if docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
    if docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        echo -e "${YELLOW}⚠️  Container '$CONTAINER_NAME' is already running${NC}"
        echo "Use: $0 stop   # to stop it"
        echo "Use: $0 logs   # to view logs"
        echo "Use: $0 -r     # to remove and recreate"
        exit 1
    else
        echo -e "${YELLOW}⚠️  Container '$CONTAINER_NAME' exists but is stopped${NC}"
        echo "Starting existing container..."
        docker start "$CONTAINER_NAME"
        if [[ "$FOLLOW_LOGS" == "true" ]]; then
            docker logs -f "$CONTAINER_NAME"
        fi
        exit 0
    fi
fi

echo -e "${GREEN}Configuration:${NC}"
echo "  • Image: $IMAGE_NAME"
echo "  • Container: $CONTAINER_NAME"
echo "  • .env file: $(pwd)/.env"
echo "  • Config dir: $(pwd)/config"
echo "  • Detached: $DETACH"
echo ""

# Build Docker run command
DOCKER_CMD="docker run"

if [[ "$DETACH" == "true" ]]; then
    DOCKER_CMD="$DOCKER_CMD -d"
else
    DOCKER_CMD="$DOCKER_CMD -it"
fi

DOCKER_CMD="$DOCKER_CMD --name $CONTAINER_NAME"
DOCKER_CMD="$DOCKER_CMD --env-file .env"
DOCKER_CMD="$DOCKER_CMD -v $(pwd)/config:/app/config:ro"
DOCKER_CMD="$DOCKER_CMD --rm"
DOCKER_CMD="$DOCKER_CMD $IMAGE_NAME"

echo -e "${YELLOW}Running command:${NC}"
echo "$DOCKER_CMD"
echo ""

# Execute the command
if eval "$DOCKER_CMD"; then
    if [[ "$DETACH" == "true" ]]; then
        echo -e "${GREEN}✅ Container started successfully in detached mode${NC}"
        echo ""
        echo -e "${BLUE}Useful commands:${NC}"
        echo "  $0 logs      # View logs"
        echo "  $0 stop      # Stop container"
        echo "  $0 exec      # Execute shell"
        echo ""

        if [[ "$FOLLOW_LOGS" == "true" ]]; then
            echo -e "${YELLOW}📋 Following logs...${NC}"
            sleep 1
            docker logs -f "$CONTAINER_NAME"
        fi
    fi
else
    echo -e "${RED}❌ Failed to start container${NC}"
    exit 1
fi
