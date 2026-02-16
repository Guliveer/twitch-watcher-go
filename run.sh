#!/usr/bin/env bash
set -euo pipefail

# Build and run twitch-miner-go
# Usage: ./scripts/run.sh [flags]
# Example: ./scripts/run.sh -config configs -port 8080 -log-level debug

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_DIR"

echo "Building twitch-miner..."
go build -o twitch-miner ./cmd/miner

echo "Starting twitch-miner..."
./twitch-miner "$@"
