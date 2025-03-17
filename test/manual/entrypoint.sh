#!/bin/sh

if [ "$1" = "lobby" ]; then
    echo "Starting lobby service..."
    exec /app/lobby
elif [ "$1" = "matcher" ]; then
    echo "Starting matcher service..."
    exec /app/matcher
else
    echo "Please specify either 'lobby' or 'matcher'"
    exit 1
fi
