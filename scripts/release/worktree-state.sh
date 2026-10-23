#!/bin/sh
# Print the provenance work-tree state of a checkout as a JSON literal.
#
#   sh scripts/release/worktree-state.sh DIR
#
# Prints "false" when DIR is a clean Git work tree, "true" when it has tracked
# changes or untracked files (ignored files such as dist/ do not count), and
# "null" when git is unavailable, DIR is not inside a work tree, or the status
# cannot be read. An untracked source file is compiled into a build, so it
# makes the tree dirty.
set -eu

[ "$#" -eq 1 ] || {
	printf 'usage: worktree-state.sh DIR\n' >&2
	exit 2
}
dir=$1

if ! command -v git >/dev/null 2>&1; then
	echo null
	exit 0
fi
if [ "$(git -C "$dir" rev-parse --is-inside-work-tree 2>/dev/null || true)" != true ]; then
	echo null
	exit 0
fi
if ! status=$(git -C "$dir" status --porcelain --untracked-files=normal 2>/dev/null); then
	echo null
	exit 0
fi
if [ -z "$status" ]; then echo false; else echo true; fi
