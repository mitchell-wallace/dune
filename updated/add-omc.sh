#!/usr/bin/env bash
set -euo pipefail

echo "== Installing Oh My ClaudeCode =="

# 1. Add marketplace (idempotent)
claude plugin marketplace add https://github.com/Yeachan-Heo/oh-my-claudecode || true

# 2. Install plugin (idempotent)
claude plugin install oh-my-claudecode || true

# 3. Run setup wizard non-interactively
claude -p "/oh-my-claudecode:omc-setup" || true

echo "== Done =="
