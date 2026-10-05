#!/usr/bin/env bash
# SessionStart hook (cloud only): put the toolchains installed by setup.sh on
# PATH for Claude's shell, ahead of the image's older /usr/local/go. Writes to
# $CLAUDE_ENV_FILE; does nothing locally.
[ "${CLAUDE_CODE_REMOTE:-}" = "true" ] || exit 0
[ -n "${CLAUDE_ENV_FILE:-}" ] || exit 0
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"
load_env
{
  echo "export PATH=\"$PATH\""
  echo "export GOTOOLCHAIN=auto FLUTTER_SUPPRESS_ANALYTICS=true DART_SUPPRESS_ANALYTICS=true"
  echo "export DATABASE_URL='$DATABASE_URL'"
  [ -n "${CHROME_EXECUTABLE:-}" ] && echo "export CHROME_EXECUTABLE='$CHROME_EXECUTABLE'"
  [ -n "${PROMPT_SCRUB_BIN:-}" ] && echo "export PROMPT_SCRUB_BIN='$PROMPT_SCRUB_BIN'"
} >> "$CLAUDE_ENV_FILE"
exit 0
