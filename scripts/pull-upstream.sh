#!/usr/bin/env bash
# Pull upstream changes into the Blackhand branch without losing ours.
#
# The Blackhand branch is a rebase of upstream/main: our commits sit on top of
# upstream's, one commit per concern, so a rebase replays them in order and
# stops at the first conflict instead of merging two histories.
#
# Why rebase and not merge: a merge would keep upstream's commit history and
# ours side by side forever, and every future pull would re-introduce the same
# conflict points. A rebase replays our (small) commits on top of upstream's
# (large) history, so the diff stays ours-vs-upstream.
#
# Usage:
#   scripts/pull-upstream.sh            # fetch, rebase, report
#   scripts/pull-upstream.sh --no-test  # skip the test run
#
# When a conflict happens, the script stops and prints what to do. The
# resolution is usually "keep both": upstream added a route next to ours, or
# renamed a binary. See docs/UPSTREAM-SYNC.md for the per-file rules.

set -euo pipefail

cd "$(dirname "$0")/.."

UPSTREAM="${UPSTREAM:-upstream}"
BRANCH="${BRANCH:-main}"
RUN_TESTS=1

for arg in "$@"; do
  case "$arg" in
    --no-test) RUN_TESTS=0 ;;
    *) echo "unknown argument: $arg" >&2; exit 2 ;;
  esac
done

if ! git rev-parse --verify "$UPSTREAM" >/dev/null 2>&1; then
  echo "no '$UPSTREAM' remote. Add it with:" >&2
  echo "  git remote add $UPSTREAM https://github.com/luqman-v1/9router-go.git" >&2
  exit 2
fi

echo "==> fetching $UPSTREAM"
git fetch "$UPSTREAM"

behind=$(git rev-list --count "HEAD..$UPSTREAM/main")
ahead=$(git rev-list --count "$UPSTREAM/main..HEAD")
echo "    behind: $behind   ahead: $ahead"

if [ "$behind" -eq 0 ]; then
  echo "==> already up to date with $UPSTREAM/main"
  exit 0
fi

echo "==> rebasing $BRANCH onto $UPSTREAM/main"
if ! git rebase "$UPSTREAM/main"; then
  echo >&2
  echo "CONFLICT. To see what is unresolved:" >&2
  echo "  git status" >&2
  echo "Resolve each file, then:" >&2
  echo "  git add <file>" >&2
  echo "  git rebase --continue" >&2
  echo >&2
  echo "To abandon the pull and keep the current state:" >&2
  echo "  git rebase --abort" >&2
  echo >&2
  echo "See docs/UPSTREAM-SYNC.md for the per-file resolution rules." >&2
  exit 1
fi

echo "==> rebased cleanly"

if [ "$RUN_TESTS" -eq 1 ]; then
  echo "==> verifying"
  # The Go toolchain defaults to 386 on some Windows installs, and the
  # upstream code overflows an int there. Force the release architecture.
  export CGO_ENABLED=0
  export GOARCH="${GOARCH:-amd64}"
  go build ./... || { echo "build failed after the rebase" >&2; exit 1; }
  go test ./internal/... || { echo "tests failed after the rebase" >&2; exit 1; }
  echo "==> build and tests pass"
fi

echo
echo "Next: review the result, then push the branch."
