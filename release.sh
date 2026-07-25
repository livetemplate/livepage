#!/usr/bin/env bash
set -euo pipefail

# Tinkerdown Release Script
# Usage: ./release.sh <version>
# Example: ./release.sh 1.0.0

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

usage() {
    echo "Usage: $0 <version>"
    echo ""
    echo "Creates a new CLI release by tagging the repository."
    echo "The tag will trigger the cli-release.yml workflow."
    echo ""
    echo "Examples:"
    echo "  $0 1.0.0    # Creates tag v1.0.0"
    echo "  $0 1.0.1    # Creates tag v1.0.1"
    echo ""
    echo "For desktop releases, use:"
    echo "  git tag desktop-v1.0.0 && git push origin desktop-v1.0.0"
    exit 1
}

# Check arguments
if [ $# -ne 1 ]; then
    usage
fi

VERSION="$1"

# Validate version format (semver without v prefix)
if ! [[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$ ]]; then
    echo -e "${RED}Error: Invalid version format. Expected: X.Y.Z or X.Y.Z-suffix${NC}"
    echo "Examples: 1.0.0, 1.0.1, 2.0.0-beta.1"
    exit 1
fi

TAG="v${VERSION}"

# Check for uncommitted changes
if ! git diff-index --quiet HEAD --; then
    echo -e "${RED}Error: You have uncommitted changes. Please commit or stash them first.${NC}"
    git status --short
    exit 1
fi

# Check if we're on main branch
CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD)
if [ "$CURRENT_BRANCH" != "main" ]; then
    echo -e "${YELLOW}Warning: You are on branch '$CURRENT_BRANCH', not 'main'.${NC}"
    read -p "Continue anyway? (y/N) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 1
    fi
fi

# Check if tag already exists
if git rev-parse "$TAG" >/dev/null 2>&1; then
    echo -e "${RED}Error: Tag $TAG already exists.${NC}"
    exit 1
fi

# Fetch latest from remote
echo -e "${YELLOW}Fetching latest from origin...${NC}"
git fetch origin

# Check if local is up to date with remote
LOCAL=$(git rev-parse HEAD)
REMOTE=$(git rev-parse origin/main 2>/dev/null || echo "")
if [ -n "$REMOTE" ] && [ "$LOCAL" != "$REMOTE" ]; then
    echo -e "${YELLOW}Warning: Local branch differs from origin/main.${NC}"
    read -p "Continue anyway? (y/N) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 1
    fi
fi

# Show what we're about to do
echo ""
echo -e "${GREEN}Release Summary:${NC}"
echo "  Version: $VERSION"
echo "  Tag:     $TAG"
echo "  Commit:  $(git rev-parse --short HEAD)"
echo "  Message: $(git log -1 --pretty=%s)"
echo ""

read -p "Create and push tag $TAG? (y/N) " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Aborted."
    exit 1
fi

# Create and push tag
echo -e "${YELLOW}Creating tag $TAG...${NC}"
git tag -a "$TAG" -m "Release $VERSION"

echo -e "${YELLOW}Pushing tag to origin...${NC}"
git push origin "$TAG"

echo ""
echo -e "${GREEN}Success! Tag $TAG has been pushed.${NC}"
echo ""
echo "The CLI release workflow will now run automatically."
echo "Monitor the release at: https://github.com/livetemplate/tinkerdown/actions"
echo ""
echo "After the release completes, update the Homebrew formula:"
echo "  1. Get SHA256 from the release checksums"
echo "  2. Update livetemplate/homebrew-tap with new version and hashes"
