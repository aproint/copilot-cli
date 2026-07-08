#!/usr/bin/env bash
# Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

if [ "$#" -lt 1 ]; then
  echo "usage: $0 <version> [source-dir] [destination-dir]" >&2
  exit 2
fi

version="$1"
source_dir="${2:-bin/local}"
destination_dir="${3:-dist/release}"

if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-.+][0-9A-Za-z.-]+)?$ ]]; then
  echo "release version must look like a v-prefixed semver tag, got: $version" >&2
  exit 2
fi

require_file() {
  local path="$1"
  if [ ! -f "$path" ]; then
    echo "required build output is missing: $path" >&2
    exit 1
  fi
}

copy_asset() {
  local source="$1"
  local name="$2"

  cp "$source" "$destination_dir/$name"
  chmod 0755 "$destination_dir/$name"
}

linux_amd64="$source_dir/copilot-linux-amd64"
linux_arm64="$source_dir/copilot-linux-arm64"
darwin_amd64="$source_dir/copilot-darwin-amd64"
darwin_arm64="$source_dir/copilot-darwin-arm64"
windows_amd64="$source_dir/copilot.exe"

require_file "$linux_amd64"
require_file "$linux_arm64"
require_file "$darwin_amd64"
require_file "$darwin_arm64"
require_file "$windows_amd64"

rm -rf "$destination_dir"
mkdir -p "$destination_dir"

copy_asset "$linux_amd64" "copilot-linux-amd64"
copy_asset "$linux_arm64" "copilot-linux-arm64"
copy_asset "$darwin_amd64" "copilot-darwin-amd64"
copy_asset "$darwin_arm64" "copilot-darwin-arm64"
copy_asset "$windows_amd64" "copilot-windows-amd64.exe"

copy_asset "$linux_amd64" "copilot-linux"
copy_asset "$darwin_amd64" "copilot-darwin"
copy_asset "$windows_amd64" "copilot-windows.exe"

copy_asset "$linux_amd64" "copilot-linux-amd64-$version"
copy_asset "$linux_arm64" "copilot-linux-arm64-$version"
copy_asset "$darwin_amd64" "copilot-darwin-amd64-$version"
copy_asset "$darwin_arm64" "copilot-darwin-arm64-$version"
copy_asset "$windows_amd64" "copilot-windows-amd64-$version.exe"

copy_asset "$linux_amd64" "copilot-linux-$version"
copy_asset "$darwin_amd64" "copilot-darwin-$version"
copy_asset "$windows_amd64" "copilot-windows-$version.exe"

find "$destination_dir" -maxdepth 1 -type f | sort
