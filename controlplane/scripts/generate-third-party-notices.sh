#!/bin/sh
set -eu

output=${1:?usage: generate-third-party-notices.sh OUTPUT}
case "$output" in
  /*) ;;
  *) output="$(pwd)/$output" ;;
esac
tmp="${output}.tmp"
modules_raw="${output}.modules.raw"
modules="${output}.modules"
# shellcheck disable=SC1007
module_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

cleanup() {
  rm -f "$tmp" "$modules_raw" "$modules"
}
trap cleanup EXIT HUP INT TERM

mkdir -p "$(dirname "$output")"
cd "$module_root"

go list -deps \
  -f '{{with .Module}}{{if not .Main}}{{.Path}}|{{.Version}}|{{.Dir}}{{end}}{{end}}' \
  ./cmd/controlplane >"$modules_raw"
sort -u "$modules_raw" >"$modules"

{
  echo "Agent Fabric control-plane third-party notices"
  echo
  echo "This file is generated from the packages linked into cmd/controlplane."
  echo "The Agent Fabric source itself is licensed separately under AGPL-3.0-only."
  echo

  echo "======================================================================"
  echo "Go toolchain and standard library"
  echo "======================================================================"
  echo
  cat "$(go env GOROOT)/LICENSE"
  if [ -f "$(go env GOROOT)/PATENTS" ]; then
    echo
    echo "--- PATENTS ---"
    cat "$(go env GOROOT)/PATENTS"
  fi

  while IFS='|' read -r module version directory; do
        [ -n "$module" ] || continue
        echo
        echo "======================================================================"
        echo "$module $version"
        echo "======================================================================"
        found=false
        for license in \
          "$directory"/LICENSE* \
          "$directory"/COPYING* \
          "$directory"/NOTICE*; do
          [ -f "$license" ] || continue
          found=true
          echo
          echo "--- $(basename "$license") ---"
          cat "$license"
        done
        if [ "$found" = false ]; then
          echo "ERROR: no license or notice file found for $module $version" >&2
          exit 1
        fi
      done <"$modules"
} >"$tmp"

mv "$tmp" "$output"
