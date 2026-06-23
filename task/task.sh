#!/usr/bin/env bash
# Invoked by the Azure DevOps agent on Linux/macOS. The SYSTEM_* and BUILD_*
# pipeline variables are already in the environment; OPENAI_API_KEY must be set
# as a secret pipeline variable and mapped in via the task's env block.
set -euo pipefail

TASK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BINARY="$TASK_DIR/bin/linux-amd64/ai-code-review"

if [[ ! -x "$BINARY" ]]; then
    echo "##[error]Binary not found: $BINARY (build it with: make build-linux)"
    exit 1
fi

# Map task inputs to the environment variables the binary reads.
export FAIL_ON="${INPUT_FAILONSEVERITY:-}"
export CONFIG_PATH="${INPUT_CONFIGPATH:-.codereview.yml}"

exec "$BINARY"
