#!/usr/bin/env bash
# Run the reviewer locally against the diff between HEAD and a target branch.
#
# Defaults to MOCK_AI=1 and DRY_RUN=1, so it needs no API key and posts nothing.
# To exercise the real OpenAI call without posting to Azure DevOps:
#
#   OPENAI_API_KEY=sk-... MOCK_AI=0 ./scripts/local-test.sh
#
# Usage: ./scripts/local-test.sh [target-branch]   (default: main)
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TARGET_BRANCH="${1:-main}"

# Load a local .env if present (developer convenience; never used in CI).
if [[ -f "$REPO_ROOT/.env" ]]; then
    set -a
    # shellcheck disable=SC1091
    source "$REPO_ROOT/.env"
    set +a
fi

if ! git -C "$REPO_ROOT" rev-parse HEAD >/dev/null 2>&1; then
    echo "error: this repo has no commits yet"
    exit 1
fi

# Fall back to a local branch when origin/<branch> isn't available.
if ! git -C "$REPO_ROOT" rev-parse "origin/$TARGET_BRANCH" >/dev/null 2>&1; then
    if git -C "$REPO_ROOT" rev-parse "$TARGET_BRANCH" >/dev/null 2>&1; then
        echo "note: origin/$TARGET_BRANCH not found, using local branch '$TARGET_BRANCH'"
        git -C "$REPO_ROOT" update-ref "refs/remotes/origin/$TARGET_BRANCH" \
            "$(git -C "$REPO_ROOT" rev-parse "$TARGET_BRANCH")"
    else
        echo "error: neither origin/$TARGET_BRANCH nor local branch '$TARGET_BRANCH' exists"
        exit 1
    fi
fi

echo "building..."
(cd "$REPO_ROOT" && go build -o /tmp/ai-code-review .)

echo "reviewing diff against $TARGET_BRANCH"
echo "----------------------------------------"

DRY_RUN="${DRY_RUN:-1}" \
MOCK_AI="${MOCK_AI:-1}" \
SYSTEM_PULLREQUEST_PULLREQUESTID=999 \
SYSTEM_PULLREQUEST_TARGETBRANCHNAME="$TARGET_BRANCH" \
BUILD_REPOSITORY_LOCALPATH="$REPO_ROOT" \
SYSTEM_TEAMFOUNDATIONCOLLECTIONURI="https://dev.azure.com/local-test" \
SYSTEM_TEAMPROJECT="local-test" \
BUILD_REPOSITORY_ID="00000000-0000-0000-0000-000000000000" \
SYSTEM_ACCESSTOKEN="dry-run" \
OPENAI_API_KEY="${OPENAI_API_KEY:-dry-run}" \
/tmp/ai-code-review
