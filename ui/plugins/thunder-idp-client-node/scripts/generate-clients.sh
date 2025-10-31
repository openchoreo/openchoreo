#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Get the directory where this script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PLUGIN_DIR="$(dirname "$SCRIPT_DIR")"
OPENAPI_DIR="$PLUGIN_DIR/openapi"
SRC_DIR="$PLUGIN_DIR/src"
GENERATED_DIR="$SRC_DIR/generated"

echo -e "${BLUE}🔧 Thunder IdP API Client Generator${NC}"
echo ""

# Parse command line arguments
THUNDER_VERSION=""
while [[ $# -gt 0 ]]; do
  case $1 in
    --thunder-version)
      THUNDER_VERSION="$2"
      shift 2
      ;;
    *)
      echo -e "${RED}❌ Unknown argument: $1${NC}"
      echo "Usage: $0 [--thunder-version v0.10.0]"
      exit 1
      ;;
  esac
done

# If no version provided via CLI, read from package.json
if [ -z "$THUNDER_VERSION" ]; then
  echo -e "${YELLOW}📋 Reading Thunder version from package.json...${NC}"

  # Check if node is available
  if ! command -v node &> /dev/null; then
    echo -e "${RED}❌ Node.js is required but not found${NC}"
    exit 1
  fi

  # Read thunderVersion from package.json
  THUNDER_VERSION=$(node -pe "require('$PLUGIN_DIR/package.json').thunderVersion || ''" 2>/dev/null)

  if [ -z "$THUNDER_VERSION" ]; then
    echo -e "${RED}❌ thunderVersion not found in package.json${NC}"
    echo "Please add 'thunderVersion' field to package.json or provide --thunder-version argument"
    exit 1
  fi
fi

echo -e "${GREEN}✓ Using Thunder version: ${THUNDER_VERSION}${NC}"

# Validate version format (should be vX.Y.Z)
if [[ ! "$THUNDER_VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+.*$ ]]; then
  echo -e "${YELLOW}⚠️  Warning: Version format should be vX.Y.Z (e.g., v0.10.0)${NC}"
fi

# Create directories
echo ""
echo -e "${YELLOW}📁 Creating directories...${NC}"
mkdir -p "$OPENAPI_DIR"
mkdir -p "$GENERATED_DIR/user"
mkdir -p "$GENERATED_DIR/group"

# Download OpenAPI specs
echo ""
echo -e "${YELLOW}⬇️  Downloading OpenAPI specifications...${NC}"

USER_SPEC_URL="https://raw.githubusercontent.com/asgardeo/thunder/refs/tags/${THUNDER_VERSION}/docs/apis/user.yaml"
GROUP_SPEC_URL="https://raw.githubusercontent.com/asgardeo/thunder/refs/tags/${THUNDER_VERSION}/docs/apis/group.yaml"

echo -e "   User API:  ${USER_SPEC_URL}"
echo -e "   Group API: ${GROUP_SPEC_URL}"

# Download user.yaml
if curl -fsSL "$USER_SPEC_URL" -o "$OPENAPI_DIR/user.yaml"; then
  echo -e "${GREEN}✓ Downloaded user.yaml${NC}"
else
  echo -e "${RED}❌ Failed to download user.yaml${NC}"
  echo "Please check if the version ${THUNDER_VERSION} exists in the Thunder repository"
  exit 1
fi

# Download group.yaml
if curl -fsSL "$GROUP_SPEC_URL" -o "$OPENAPI_DIR/group.yaml"; then
  echo -e "${GREEN}✓ Downloaded group.yaml${NC}"
else
  echo -e "${RED}❌ Failed to download group.yaml${NC}"
  exit 1
fi

# Generate version.ts file
echo ""
echo -e "${YELLOW}📝 Generating version.ts...${NC}"
cat > "$SRC_DIR/version.ts" << EOF
/**
 * Thunder IdP API Version
 * Auto-generated from package.json thunderVersion field
 * DO NOT EDIT MANUALLY
 *
 * @packageDocumentation
 */

export const THUNDER_VERSION = '${THUNDER_VERSION}';
EOF
echo -e "${GREEN}✓ Generated version.ts${NC}"

# Check if openapi-typescript is available
echo ""
echo -e "${YELLOW}🔨 Generating TypeScript types from OpenAPI specs...${NC}"

# Generate User API types
echo -e "   Generating User API types..."
npx openapi-typescript "$OPENAPI_DIR/user.yaml" -o "$GENERATED_DIR/user/types.ts"

if [ $? -eq 0 ]; then
  echo -e "${GREEN}✓ User API types generated successfully${NC}"
else
  echo -e "${RED}❌ Failed to generate User API types${NC}"
  exit 1
fi

# Generate Group API types
echo -e "   Generating Group API types..."
npx openapi-typescript "$OPENAPI_DIR/group.yaml" -o "$GENERATED_DIR/group/types.ts"

if [ $? -eq 0 ]; then
  echo -e "${GREEN}✓ Group API types generated successfully${NC}"
else
  echo -e "${RED}❌ Failed to generate Group API types${NC}"
  exit 1
fi

# Create index files for each API
echo ""
echo -e "${YELLOW}📝 Creating index files...${NC}"

# User API index
cat > "$GENERATED_DIR/user/index.ts" << 'EOF'
/**
 * Thunder User Management API Client
 * Auto-generated TypeScript types from OpenAPI spec
 *
 * @packageDocumentation
 */

export * from './types';
EOF

# Group API index
cat > "$GENERATED_DIR/group/index.ts" << 'EOF'
/**
 * Thunder Group Management API Client
 * Auto-generated TypeScript types from OpenAPI spec
 *
 * @packageDocumentation
 */

export * from './types';
EOF

echo -e "${GREEN}✓ Index files created${NC}"

# Summary
echo ""
echo -e "${GREEN}✅ API client generation completed successfully!${NC}"
echo ""
echo -e "${BLUE}📦 Generated clients:${NC}"
echo -e "   User API:  ${GENERATED_DIR}/user"
echo -e "   Group API: ${GENERATED_DIR}/group"
echo -e "   Version:   ${SRC_DIR}/version.ts"
echo ""
echo -e "${BLUE}💡 Next steps:${NC}"
echo -e "   1. Review generated clients in src/generated/"
echo -e "   2. Run 'yarn build' to compile the package"
echo -e "   3. Import and use the clients in your code"
echo ""
