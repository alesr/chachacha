# ChaChaCha Matchmaking Test Client

This directory contains tools for manually testing the matchmaking system.

## Running Tests

```bash
# Start the Refis, RabbitMQ, the lobby, matcher and the test client services
docker compose -f test/manual/docker-compose.yml up -d

# See lobby logs with hosts and players registrations
docker-compose -f test/manual/docker-compose.yml logs -f lobby

# See matcher logs with matchmaking information between hosts and players
docker-compose -f test/manual/docker-compose.yml logs -f matcher

# To start the test TUI
./test/manual/run-test.sh
```

Access the RabbitMQ dashboard at `http://localhost:15672/` to see the created events.
