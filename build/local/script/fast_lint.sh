#!/usr/bin/env bash

original_dir=$(pwd)

set -euo pipefail
cd "$( dirname "${BASH_SOURCE[0]}" )"

PACKAGES_CHANGED_SINCE_MASTER=$(./find_packages_changed_since_master.py)


if [[ "${PACKAGES_CHANGED_SINCE_MASTER}" != "" ]]; then
    pushd $original_dir 1>&2 2>/dev/null
    echo "linting ${PACKAGES_CHANGED_SINCE_MASTER}"
    bazel run //:gometalinterfast -- --fast ${PACKAGES_CHANGED_SINCE_MASTER}
    popd 1>&2 2>/dev/null
else
    echo "no go packages found to lint"
fi
