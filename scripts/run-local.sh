#!/bin/sh
set -eu

: "${INFRAI_API_KEY:?set INFRAI_API_KEY first}"
go run ./cmd/avatar-service
