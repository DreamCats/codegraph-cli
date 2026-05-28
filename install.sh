#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"
go install ./cmd/codegraph

echo "installed codegraph via go install ./cmd/codegraph"
