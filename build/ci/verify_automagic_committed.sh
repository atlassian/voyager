#!/usr/bin/env bash

set -euo pipefail

make check-all-automagic-changes-were-commited-before-ci
touch success
