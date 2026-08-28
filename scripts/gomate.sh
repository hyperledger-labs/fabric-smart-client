#!/usr/bin/env bash
#
# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0
#

set -euo pipefail

# lint me: shfmt -i 2 -ci -l -w

IFS=$'\t\n' # Split on newlines and tabs (but not on spaces)
script_name=$(basename "${0}")
script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
readonly script_name script_dir

readonly repo_dir="$(git rev-parse --show-toplevel)"

# List the directory of every go module tracked in this repository, one per line.
#
# Discovery goes through git rather than a filesystem walk: a plain `find` also
# descends into untracked scratch directories and into nested git worktrees
# checked out under the repo (e.g. .claude/worktrees/<branch>), where `go mod
# tidy` / `go get` would silently rewrite another branch's go.mod and go.sum.
function module_dirs() {
  git -C "$repo_dir" ls-files | grep -E '(^|/)go\.mod$' | while IFS= read -r gomod; do
    printf '%s\n' "$repo_dir/$(dirname "$gomod")"
  done
}

# run a command in every module directory
function in_each_module() {
  local dir
  for dir in $(module_dirs); do
    echo "  -> ${dir#"$repo_dir"/}"
    (cd "$dir" && "$@")
  done
}

# how to use this script
function usage() {
  cli_name=${0##*/}
  echo "gomate helps to manage multiple go modules in a single repository.
Usage: $cli_name [command]
Commands:
  initwork      creates go workspace for this project (\`go work init\` and \`go work use\` everywhere)
  tidy          runs \`go mod tidy\` everywhere
  update [XYZ]  updates a specific dep (\`go get XYZ\`) everywhere; if no dep argument given, \`go get -u\` is called
  help          shows this help
  "
  exit 1
}

# create go work init
function init_work() {
  echo "go work init"
  go work init
  # shellcheck disable=SC2046 # module_dirs emits one path per line, IFS is \t\n
  go work use $(module_dirs)
}

# update deps; take as parameter the dependency to update; if empty all deps are updates
function update() {
  if [[ -z $1 ]]; then
    # check all update
    echo "go get -u everywhere"
    in_each_module go get -u
  else
    # check a specific dep
    echo "go get $1 everywhere"
    in_each_module go get "$1"
  fi
}

# run go mod tidy everywhere
function tidy() {
  echo "go mod tidy everywhere"
  in_each_module go mod tidy
}

main() {
  case "${1-""}" in
    initwork)
      init_work
      ;;
    tidy)
      tidy
      ;;
    update)
      update "${2-""}"
      ;;
    *)
      usage
      ;;
  esac
}

main "${@}"
