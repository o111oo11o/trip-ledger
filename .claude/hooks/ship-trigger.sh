#!/usr/bin/env bash
# .claude/hooks/ship-trigger.sh
#
# Fires on every Stop event. Reads the transcript to check if Claude's
# last message contains the FEATURE_COMPLETE marker. If it does, forces
# Claude to continue and run /ship.
#
# Marker: Claude must output the exact string FEATURE_COMPLETE somewhere
# in its response when it finishes implementing a feature. See CLAUDE.md.

set -euo pipefail

# Read the hook event JSON from stdin
INPUT=$(cat)

# Extract transcript path
TRANSCRIPT_PATH=$(echo "$INPUT" | python3 -c "import sys, json; print(json.load(sys.stdin).get('transcript_path', ''))" 2>/dev/null || echo "")

if [[ -z "$TRANSCRIPT_PATH" || ! -f "$TRANSCRIPT_PATH" ]]; then
  exit 0
fi

# Get the last assistant message from the JSONL transcript
LAST_ASSISTANT_MSG=$(python3 - <<'EOF'
import sys, json

transcript_path = sys.argv[1]
last_msg = ""

with open(transcript_path, "r") as f:
    for line in f:
        line = line.strip()
        if not line:
            continue
        try:
            entry = json.loads(line)
        except json.JSONDecodeError:
            continue
        # Transcript entries have a "role" field
        if entry.get("role") == "assistant":
            # Content can be a string or list of blocks
            content = entry.get("content", "")
            if isinstance(content, str):
                last_msg = content
            elif isinstance(content, list):
                parts = []
                for block in content:
                    if isinstance(block, dict) and block.get("type") == "text":
                        parts.append(block.get("text", ""))
                last_msg = "\n".join(parts)
EOF
"$TRANSCRIPT_PATH"
)

# Check for the marker
if echo "$LAST_ASSISTANT_MSG" | grep -q "FEATURE_COMPLETE"; then
  # Force Claude to continue and run /ship
  echo '{"continue": true, "reason": "Feature implementation complete. Run /ship now to simplify, test, deploy, and update CLAUDE.md."}'
  exit 0
fi

exit 0
