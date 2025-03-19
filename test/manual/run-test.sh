#!/bin/bash

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[0;33m'
NC='\033[0m' # no color

echo -e "${BLUE}=== ChaChaChá Matchmaking Test Tool ===${NC}"

# make sure services are running
if ! docker-compose -f test/manual/docker-compose.yml ps | grep -q "Up"; then
    echo -e "${RED}Services are not running!${NC}"
    echo -e "Please start the services first with:"
    echo -e "${GREEN}docker-compose -f test/manual/docker-compose.yml up -d${NC}"
    exit 1
fi

# run the test client inside the container with terminal support
echo -e "${BLUE}Starting ChaChaChá interactive test client...${NC}"
docker-compose -f test/manual/docker-compose.yml exec -it test-client /app/test-client
