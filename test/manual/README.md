# ChaChaCha Matchmaking Test Client

This directory contains tools for manually testing the matchmaking system.

## Running Tests

```bash
# Start the Refis, RabbitMQ, the lobby, matcher and the test client services
docker-compose -f test/manual/docker-compose.yml up -d

# See lobby logs with hosts and players registrations
docker-compose -f test/manual/docker-compose.yml logs -f lobby

# See matcher logs with matchmaking information between hosts and players
docker-compose -f test/manual/docker-compose.yml logs -f matcher

# Use test/manual/main.go to register a host
./test/manual/run-test.sh -mode host

# Use test/manual/main.go to register a host
./test/manual/run-test.sh -mode player
```

Additionally, you can access the RabbitMQ dashboard at `http://localhost:15672/`.
