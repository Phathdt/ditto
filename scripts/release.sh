#!/bin/bash
set -e

VERSION=$1

if [ -z "$VERSION" ]; then
    echo "Usage: $0 <version>"
    echo "Example: $0 v1.0.0"
    exit 1
fi

if [[ ! $VERSION =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "Error: Version must be in format vX.Y.Z (e.g., v1.0.0)"
    exit 1
fi

# Extract repository info from git remote
REMOTE_URL=$(git config --get remote.origin.url)
if [[ $REMOTE_URL =~ github\.com[:/]([^/]+)/([^/]+)(\.git)?$ ]]; then
    REPO_OWNER="${BASH_REMATCH[1]}"
    REPO_NAME="${BASH_REMATCH[2]}"
else
    echo "Error: Could not parse GitHub repository from remote URL: $REMOTE_URL"
    exit 1
fi

echo "Repository: $REPO_OWNER/$REPO_NAME"
echo "Creating release $VERSION..."

# Check if tag already exists
if git tag -l | grep -q "^$VERSION$"; then
    echo "Error: Tag $VERSION already exists"
    exit 1
fi

# Check if working directory is clean
if [[ -n $(git status --porcelain) ]]; then
    echo "Error: Working directory is not clean. Please commit or stash your changes."
    git status --short
    exit 1
fi

# Make sure we're on the main/master branch
CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD)
if [[ "$CURRENT_BRANCH" != "main" && "$CURRENT_BRANCH" != "master" ]]; then
    echo "Warning: You are not on main/master branch (current: $CURRENT_BRANCH)"
    read -p "Continue anyway? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 1
    fi
fi

# Create and push git tag
echo "Creating tag $VERSION..."
git tag -a $VERSION -m "Release $VERSION"

echo "Pushing tag to origin..."
git push origin $VERSION

echo ""
echo "✅ Tag $VERSION created and pushed successfully!"
echo ""
echo "🚀 GitHub Actions will automatically:"
echo "   • Create the GitHub release"
echo "   • Build and push the Docker image to phathdt379/ditto:$VERSION"
echo "   • Update the latest tag"
echo ""
echo "🔗 Monitor progress: https://github.com/$REPO_OWNER/$REPO_NAME/actions"
echo "🔗 Releases: https://github.com/$REPO_OWNER/$REPO_NAME/releases"
echo "🐳 Docker Hub: https://hub.docker.com/repository/docker/phathdt379/ditto"
echo ""
echo "📦 Once built, you can pull the image with:"
echo "   docker pull phathdt379/ditto:$VERSION"
echo "   docker pull phathdt379/ditto:latest"
