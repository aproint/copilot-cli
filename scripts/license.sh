#!/bin/bash
# Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
# SPDX-License-Identifier: Apache-2.0
#
# license.sh checks that all Go files in the given correct-looking license header.

check_header() {
    got=$1
    want=$(cat <<EOF
// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0
EOF
    )
    if [[ "$got" = *"$want"* ]]; then
        return 0
    fi
    return 1
}

if [ $# != 1 ]; then
    echo "Usage: $0 rootdir" >&2
    exit 1
fi

rootdir=$1

fail=0
while IFS= read -r -d '' file; do
    case $file in
        $rootdir/*/mocks/*)
            # Skip mocks packages.
        ;;
        $rootdir/*/mock_*.go)
            # Skip mock files.
        ;;
        $rootdir/*/node_modules/*)
            # Skip node modules for js files.
        ;;
        $rootdir/*/coverage/*)
            # Skip generated coverage reports.
        ;;
        $rootdir/site/*)
            # Skip website content
        ;;
        *)
            header="$(head -10 $file)"
            if ! check_header "$header"; then
                fail=1
                echo "${file#$rootdir/} doesn't have the right copyright header:"
                echo "$header" | sed -e 's/^/    /g'
            fi
            ;;
    esac
done < <(find "$rootdir" \( -name '*.go' -o -name '*.js' \) -print0)

if [ $fail -ne 0 ]; then
    exit 1
fi
