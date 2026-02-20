#!/usr/bin/env bash
set -euo pipefail

# Build and run twitch-miner-go
# Usage: ./scripts/run.sh [flags]
# Example: ./scripts/run.sh -config configs -port 8080 -log-level debug

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_DIR"

echo "Building twitch-miner-go..."
go build -o twitch-miner-go ./cmd/twitch-miner-go

echo "Starting twitch-miner-go..."
./twitch-miner-go "$@"
