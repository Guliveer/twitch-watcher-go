#!/usr/bin/env bash
set -euo pipefail

# Build and run twitch-watcher-go
# Usage: ./scripts/run.sh [flags]
# Example: ./scripts/run.sh -config configs -port 8080 -log-level debug

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_DIR"

echo "Building twitch-watcher-go..."
go build -o twitch-watcher-go ./cmd/twitch-watcher-go

echo "Starting twitch-watcher-go..."
./twitch-watcher-go "$@"
