#!/bin/bash

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[0;33m'
NC='\033[0m' # no color

echo -e "${BLUE}=== ChaChaChá Matchmaking Test Tool ===${NC}"

if [ "$1" == "--help" ] || [ "$#" -eq 0 ]; then
    echo -e "${YELLOW}Usage:${NC} $0 [options]"
    echo ""
    echo "Options:"
    echo -e "  ${GREEN}-mode host${NC}          Register a new game host"
    echo -e "  ${GREEN}-mode player${NC}        Register a player looking for a match"
    echo -e "  ${GREEN}-mode remove-host${NC}   Remove a host from matchmaking"
    echo -e "  ${GREEN}-mode remove-player${NC} Remove a player from matchmaking"
    echo -e "  ${GREEN}-mode status${NC}        Show current matchmaking status"
    echo -e "  ${GREEN}-verbose${NC}            Show detailed event information"
    echo ""
    echo "Examples:"
    echo "  $0 -mode host           # Register a new host"
    echo "  $0 -mode player         # Register a new player"
    echo "  $0 -mode status         # Show current state"
    echo "  $0 -mode host -verbose  # Register a host with detailed event info"
    echo ""
    echo -e "${YELLOW}Workflow Example:${NC}"
    echo "  1. $0 -mode status                # See initial state"
    echo "  2. $0 -mode host                  # Add a host"
    echo "  3. $0 -mode player                # Add a player"
    echo "  4. $0 -mode status                # See the match created"
    echo "  5. $0 -mode remove-host -verbose  # Remove host with detailed output"
    echo "  6. $0 -mode status                # See that players are back in matchmaking"
    exit 1
fi

# make sure services are running
if ! docker-compose -f test/manual/docker-compose.yml ps | grep -q "Up"; then
    echo -e "${RED}Services are not running!${NC}"
    echo -e "Please start the services first with:"
    echo -e "${GREEN}docker-compose -f test/manual/docker-compose.yml up -d${NC}"
    exit 1
fi

# run the test client inside the container with the supplied arguments
echo -e "${BLUE}Running test client...${NC}"
docker-compose -f test/manual/docker-compose.yml exec test-client /app/test-client "$@"
