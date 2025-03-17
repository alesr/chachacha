#!/bin/bash

# Run the test client inside the container with the supplied arguments
echo "Running test client..."
docker-compose -f test/manual/docker-compose.yml exec test-client /app/test-client "$@"
