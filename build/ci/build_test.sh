#!/usr/bin/env bash

set -euo pipefail

make build-and-test-in-ci
touch success
