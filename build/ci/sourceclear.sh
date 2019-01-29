#!/usr/bin/env bash

set -euo pipefail

export SRCCLR_API_TOKEN="$1"
curl -sSL https://download.sourceclear.com/ci.sh | sh
