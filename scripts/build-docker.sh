#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}🐳 Docker Build Test Script${NC}"
echo "================================"

# Check if Docker is running
if ! docker info > /dev/null 2>&1; then
    echo -e "${RED}❌ Docker is not running${NC}"
    exit 1
fi

# Check if buildx is available
if ! docker buildx version > /dev/null 2>&1; then
    echo -e "${RED}❌ Docker buildx is not available${NC}"
    exit 1
fi

# Default values
PLATFORMS="linux/amd64,linux/arm64"
IMAGE_NAME="ditto-test"
PUSH=false
LOAD=false

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -p|--platforms)
            PLATFORMS="$2"
            shift 2
            ;;
        -i|--image)
            IMAGE_NAME="$2"
            shift 2
            ;;
        --push)
            PUSH=true
            shift
            ;;
        --load)
            LOAD=true
            shift
            ;;
        -h|--help)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  -p, --platforms PLATFORMS    Target platforms (default: linux/amd64,linux/arm64)"
            echo "  -i, --image IMAGE_NAME       Image name (default: ditto-test)"
            echo "  --push                        Push to registry"
            echo "  --load                        Load to local Docker"
            echo "  -h, --help                   Show this help"
            echo ""
            echo "Examples:"
            echo "  $0                           # Build for AMD64 and ARM64"
            echo "  $0 -p linux/amd64           # Build only for AMD64"
            echo "  $0 --load -p linux/amd64    # Build and load to local Docker"
            exit 0
            ;;
        *)
            echo -e "${RED}❌ Unknown option: $1${NC}"
            exit 1
            ;;
    esac
done

echo -e "${YELLOW}Build Configuration:${NC}"
echo "  • Platforms: $PLATFORMS"
echo "  • Image: $IMAGE_NAME"
echo "  • Push: $PUSH"
echo "  • Load: $LOAD"
echo ""

# Check if Dockerfile exists
if [[ ! -f "Dockerfile" ]]; then
    echo -e "${RED}❌ Dockerfile not found${NC}"
    exit 1
fi

# Check if go.mod exists
if [[ ! -f "go.mod" ]]; then
    echo -e "${RED}❌ go.mod not found${NC}"
    exit 1
fi

# Show Go version requirement
GO_VERSION=$(grep "^go " go.mod | awk '{print $2}')
echo -e "${YELLOW}Go version requirement: $GO_VERSION${NC}"

# Check current Go version in Dockerfile
DOCKERFILE_GO_VERSION=$(grep "FROM golang:" Dockerfile | head -1 | sed 's/.*golang:\([0-9.]*\).*/\1/')
echo -e "${YELLOW}Dockerfile Go version: $DOCKERFILE_GO_VERSION${NC}"

if [[ "$GO_VERSION" > "$DOCKERFILE_GO_VERSION" ]]; then
    echo -e "${RED}⚠️  WARNING: Dockerfile Go version ($DOCKERFILE_GO_VERSION) is older than required ($GO_VERSION)${NC}"
fi

echo ""

# Build command
BUILD_CMD="docker buildx build"

if [[ "$LOAD" == "true" ]]; then
    BUILD_CMD="$BUILD_CMD --load"
elif [[ "$PUSH" == "true" ]]; then
    BUILD_CMD="$BUILD_CMD --push"
fi

BUILD_CMD="$BUILD_CMD --platform $PLATFORMS"
BUILD_CMD="$BUILD_CMD -t $IMAGE_NAME"
BUILD_CMD="$BUILD_CMD ."

echo -e "${YELLOW}Running build command:${NC}"
echo "$BUILD_CMD"
echo ""

# Execute build
if eval "$BUILD_CMD"; then
    echo ""
    echo -e "${GREEN}✅ Build successful!${NC}"

    if [[ "$LOAD" == "true" ]]; then
        echo ""
        echo -e "${GREEN}🚀 Image loaded to local Docker:${NC}"
        echo "  docker run --rm $IMAGE_NAME --help"
    fi

    if [[ "$PUSH" == "true" ]]; then
        echo ""
        echo -e "${GREEN}🚀 Image pushed to registry:${NC}"
        echo "  docker pull $IMAGE_NAME"
    fi
else
    echo ""
    echo -e "${RED}❌ Build failed!${NC}"
    echo ""
    echo -e "${YELLOW}Common issues:${NC}"
    echo "  • Go version mismatch (check Dockerfile vs go.mod)"
    echo "  • Network issues during go mod download"
    echo "  • Missing dependencies in go.mod"
    echo "  • Platform-specific build issues"
    echo ""
    echo -e "${YELLOW}Debug suggestions:${NC}"
    echo "  • Run with single platform: $0 -p linux/amd64"
    echo "  • Check Docker buildx: docker buildx ls"
    echo "  • Enable debug: docker buildx build --debug ..."
    exit 1
fi
