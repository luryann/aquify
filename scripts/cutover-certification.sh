#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "Running import/export certification fixture suite"
cd "${ROOT_DIR}/internal/meetfiles"
go test ./...

echo "Running Step 23 P0 and interoperability regression coverage"
cd "${ROOT_DIR}/server"
go test ./internal/app -run 'TestP0|TestHY3ImportFinalizationAndReportGeneration|TestOpenAPIExampleContractsForCommonErrorResponses|TestSensitiveRouteRBACMatrix'
