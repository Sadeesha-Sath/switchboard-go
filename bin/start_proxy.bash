#!/usr/bin/env bash
set -e

# Change directory to the directory where this script is located
cd "$(dirname "$0")"

# Default to config.yaml in the bin folder unless SWITCHBOARD_GO_CONFIG is explicitly specified
export SWITCHBOARD_GO_CONFIG="${SWITCHBOARD_GO_CONFIG:-config.yaml}"

echo "Starting switchboard-go with config file: ${SWITCHBOARD_GO_CONFIG}"
exec ./switchboard-go "$@"