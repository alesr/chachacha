# ChaChaChá 💃🏼🕺🏼
[![codecov](https://codecov.io/gh/alesr/chachacha/graph/badge.svg?token=oAR3Ak3bhw)](https://codecov.io/gh/alesr/chachacha)

Named after the lively dance that brings partners together, ChaChaChá helps match players with game hosts—pairing them up just like dance partners on the floor.

## What is it?

ChaChaChá is a lightweight matchmaking engine for multiplayer games.
It follows an event-driven architecture and is built using Go, Redis, and RabbitMQ.
What does it do?

Using RabbitMQ events, game developers can:

- Register a host with a custom game mode and available slots.
- Register players looking to join a match, either by specifying a host ID or selecting a game mode.
- Remove hosts and players from the lobby.
- Receive notifications about various lobby events, such as all match slots being filled or new hosts and players joining the queue.

WIP – Moar documentation coming soon!
