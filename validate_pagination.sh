#!/bin/bash

# ==========================================
# OpenChoreo Pagination Validator
# Tests Server-Side Pagination (limit/continue)
# ==========================================

# Configuration
API_URL="http://localhost:8088/api/v1"
PAGE_LIMIT=2           # Small limit to force pagination even on small datasets
SAFETY_MAX_PAGES=50    # Stop after N pages to prevent infinite loops during testing

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

echo -e "${YELLOW}Starting Pagination Validation (v5 - K8s Style)...${NC}"

# Check dependencies
if ! command -v jq &> /dev/null; then
    echo -e "${RED}Error: 'jq' is not installed.${NC}"
    exit 1
fi

# Setup temp files
TEMP_DIR="/tmp/pagination_test_$$"
mkdir -p "$TEMP_DIR"
ITEMS_FILE="$TEMP_DIR/items_seen.txt"
TOKENS_FILE="$TEMP_DIR/tokens_seen.txt"
trap 'rm -rf "$TEMP_DIR"' EXIT

# ==========================================
# Helper Functions
# ==========================================

check_api_health() {
    echo -n "Checking API connectivity... "
    if curl -s "$API_URL/orgs?limit=1" > /dev/null; then
        echo -e "${GREEN}OK${NC}"
    else
        echo -e "${RED}FAILED - Is the API server running on $API_URL?${NC}"
        exit 1
    fi
}

# Generic Pagination Test Function
test_pagination() {
    local ENDPOINT_NAME=$1
    local URL=$2

    echo -e "\n${CYAN}--- TESTING: $ENDPOINT_NAME ---${NC}"
    echo "URL: $URL"
    
    CONTINUE_TOKEN=""
    PREV_TOKEN="INITIAL_START"
    PAGE_COUNT=0
    TOTAL_ITEMS=0
    DUPLICATE_ITEMS=0
    
    # Clear tracking
    > "$ITEMS_FILE"
    > "$TOKENS_FILE"

    while true; do
        ((PAGE_COUNT++))
        
        # Build Request URL
        REQ_URL="$URL?limit=$PAGE_LIMIT"
        if [ -n "$CONTINUE_TOKEN" ]; then
            # URL Encode the token using jq
            ENCODED_TOKEN=$(jq -rn --arg x "$CONTINUE_TOKEN" '$x|@uri')
            REQ_URL="${REQ_URL}&continue=${ENCODED_TOKEN}"
        fi

        # Execute Request
        RESPONSE=$(curl -s "$REQ_URL")

        # Validate JSON
        if ! echo "$RESPONSE" | jq -e . >/dev/null 2>&1; then
            echo -e "${RED}FAIL: Invalid JSON on page $PAGE_COUNT${NC}"
            return 1
        fi

        # Check for API Error
        SUCCESS=$(echo "$RESPONSE" | jq -r '.success')
        if [ "$SUCCESS" == "false" ]; then
             ERR_MSG=$(echo "$RESPONSE" | jq -r '.error // "Unknown error"')
             echo -e "${RED}FAIL: API returned error: $ERR_MSG${NC}"
             return 1
        fi

        # Extract Metadata (New API Structure)
        ITEMS_COUNT=$(echo "$RESPONSE" | jq '.data.items | length')
        HAS_MORE=$(echo "$RESPONSE" | jq -r '.data.metadata.hasMore')
        CONTINUE_TOKEN=$(echo "$RESPONSE" | jq -r '.data.metadata.continue // empty')
        
        TOTAL_ITEMS=$((TOTAL_ITEMS + ITEMS_COUNT))

        # Check for item duplicates
        # We assume items have a 'name' field, fall back to whole object if not
        for item in $(echo "$RESPONSE" | jq -r '.data.items[].name // "unknown"'); do
            if [ "$item" != "unknown" ]; then
                if grep -q "^${item}$" "$ITEMS_FILE" 2>/dev/null; then
                    ((DUPLICATE_ITEMS++))
                else
                    echo "$item" >> "$ITEMS_FILE"
                fi
            fi
        done

        # Cycle Detection
        if [ -n "$CONTINUE_TOKEN" ] && grep -q "^${CONTINUE_TOKEN}$" "$TOKENS_FILE" 2>/dev/null; then
             echo -e "${RED}FAIL: Token cycle detected!${NC}"
             return 1
        fi
        [ -n "$CONTINUE_TOKEN" ] && echo "$CONTINUE_TOKEN" >> "$TOKENS_FILE"

        # Log Page Status
        echo -e "   Page $PAGE_COUNT: Got $ITEMS_COUNT items | HasMore: $HAS_MORE"

        # Stop Conditions
        if [ "$HAS_MORE" == "false" ]; then
            echo -e "${GREEN}   ✓ End of list reached.${NC}"
            break
        fi

        if [ -z "$CONTINUE_TOKEN" ] && [ "$HAS_MORE" == "true" ]; then
            echo -e "${RED}FAIL: hasMore=true but continue token is missing.${NC}"
            return 1
        fi

        if [ "$CONTINUE_TOKEN" == "$PREV_TOKEN" ]; then
             echo -e "${RED}FAIL: Token did not change between pages.${NC}"
             return 1
        fi
        PREV_TOKEN="$CONTINUE_TOKEN"

        if [ "$PAGE_COUNT" -ge "$SAFETY_MAX_PAGES" ]; then
             echo -e "${YELLOW}⚠ Safety limit reached.${NC}"
             break
        fi
    done

    # Summary
    echo -e "   Total: $TOTAL_ITEMS items. Duplicates: $DUPLICATE_ITEMS"
    if [ $DUPLICATE_ITEMS -eq 0 ]; then
        echo -e "${GREEN}✅ PASSED: $ENDPOINT_NAME${NC}"
    else
        echo -e "${RED}❌ FAILED: Duplicates found${NC}"
    fi
}

# ==========================================
# Discovery Functions
# ==========================================

get_org() {
    # Get the first available organization
    local RES=$(curl -s "$API_URL/orgs?limit=1")
    echo "$RES" | jq -r '.data.items[0].name // empty'
}

get_project() {
    local ORG=$1
    # Get the first project in the org
    local RES=$(curl -s "$API_URL/orgs/$ORG/projects?limit=1")
    echo "$RES" | jq -r '.data.items[0].name // empty'
}

get_component() {
    local ORG=$1
    local PROJ=$2
    # Get the first component in the project
    local RES=$(curl -s "$API_URL/orgs/$ORG/projects/$PROJ/components?limit=1")
    echo "$RES" | jq -r '.data.items[0].name // empty'
}

# ==========================================
# Main Execution
# ==========================================

check_api_health

# 1. Test Global/Org Level Endpoints
test_pagination "Organizations" "$API_URL/orgs"

# Discover Org
ORG_NAME=$(get_org)
if [ -z "$ORG_NAME" ]; then
    echo -e "${YELLOW}No organizations found. Stopping deep tests.${NC}"
    exit 0
fi
echo -e "\n${GREEN}Targeting Org: $ORG_NAME${NC}"

# Test Org-Scoped Resources
test_pagination "Build Planes" "$API_URL/orgs/$ORG_NAME/buildplanes"
test_pagination "Data Planes" "$API_URL/orgs/$ORG_NAME/dataplanes"
test_pagination "Environments" "$API_URL/orgs/$ORG_NAME/environments"
test_pagination "Secret References" "$API_URL/orgs/$ORG_NAME/secret-references"
test_pagination "Component Types" "$API_URL/orgs/$ORG_NAME/component-types"
test_pagination "Workflows" "$API_URL/orgs/$ORG_NAME/workflows"
test_pagination "Traits" "$API_URL/orgs/$ORG_NAME/traits"
test_pagination "Component Workflows" "$API_URL/orgs/$ORG_NAME/component-workflows"
test_pagination "Projects" "$API_URL/orgs/$ORG_NAME/projects"

# 2. Test Project Level Endpoints
PROJ_NAME=$(get_project "$ORG_NAME")
if [ -z "$PROJ_NAME" ]; then
    echo -e "${YELLOW}No projects found in $ORG_NAME. Skipping project tests.${NC}"
    exit 0
fi
echo -e "\n${GREEN}Targeting Project: $PROJ_NAME${NC}"

test_pagination "Components" "$API_URL/orgs/$ORG_NAME/projects/$PROJ_NAME/components"

# 3. Test Component Level Endpoints
COMP_NAME=$(get_component "$ORG_NAME" "$PROJ_NAME")
if [ -z "$COMP_NAME" ]; then
    echo -e "${YELLOW}No components found in $PROJ_NAME. Skipping component tests.${NC}"
    exit 0
fi
echo -e "\n${GREEN}Targeting Component: $COMP_NAME${NC}"

test_pagination "Workloads" "$API_URL/orgs/$ORG_NAME/projects/$PROJ_NAME/components/$COMP_NAME/workloads"
test_pagination "Releases" "$API_URL/orgs/$ORG_NAME/projects/$PROJ_NAME/components/$COMP_NAME/component-releases"

# Release Bindings (requires at least one release, might be empty but we check the endpoint logic)
test_pagination "Release Bindings" "$API_URL/orgs/$ORG_NAME/projects/$PROJ_NAME/components/$COMP_NAME/release-bindings"
