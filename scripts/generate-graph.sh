#!/usr/bin/env bash
set -euo pipefail

# generate-graph.sh
# Generates a Terraform plan dependency graph (.dot) from a target configuration directory
# by creating a temporary copy with cloud and remote backend blocks safely replaced with
# a local backend, preventing configuration conflicts and avoiding remote network access.

TARGET_DIR="${1:-.}"
OUTPUT_DOT_FILE="${2:-graph.dot}"

if ! command -v terraform >/dev/null 2>&1; then
  echo "::error::'terraform' CLI is required to generate the dependency graph. Please install it (e.g. using hashicorp/setup-terraform)."
  exit 1
fi

if [[ ! -d "$TARGET_DIR" ]]; then
  echo "::error::Target directory '$TARGET_DIR' does not exist."
  exit 1
fi

TMP_DIR=$(mktemp -d)
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

echo "Copying Terraform configuration to temporary workspace..."
# Copy configuration to temporary directory (dereferencing symlinks if needed, ignoring hidden .terraform/.git)
tar -C "$TARGET_DIR" --exclude='.git' --exclude='.terraform' -cf - . | tar -C "$TMP_DIR" -xf -

echo "Replacing cloud and backend blocks with local backend in temporary copy..."
TMP_DIR="$TMP_DIR" python3 -c '
import os, re

def strip_cloud_and_backend(content):
    pattern = re.compile(r"(\b(?:cloud|backend\s+[\"\w]+)\s*\{)", re.MULTILINE)
    result = []
    pos = 0
    while True:
        m = pattern.search(content, pos)
        if not m:
            result.append(content[pos:])
            break
        result.append(content[pos:m.start()])
        brace_count = 1
        idx = m.end()
        while idx < len(content) and brace_count > 0:
            if content[idx] == "{":
                brace_count += 1
            elif content[idx] == "}":
                brace_count -= 1
            idx += 1
        result.append("/* stripped " + m.group(1).strip() + " ... } */\n  backend \"local\" {}\n")
        pos = idx
    return "".join(result)

tmp_dir = os.environ.get("TMP_DIR", "")
if tmp_dir:
    for root, _, files in os.walk(tmp_dir):
        for file in files:
            if file.endswith(".tf"):
                path = os.path.join(root, file)
                try:
                    with open(path, "r", encoding="utf-8", errors="ignore") as f:
                        content = f.read()
                    stripped = strip_cloud_and_backend(content)
                    if stripped != content:
                        with open(path, "w", encoding="utf-8") as f:
                            f.write(stripped)
                except Exception as e:
                    pass
' 

echo "Initializing Terraform in temporary workspace..."
(
  cd "$TMP_DIR"
  terraform init -input=false
  echo "Generating plan dependency graph with -type=plan..."
  terraform graph -type=plan > graph.dot
)

ABS_OUTPUT=$(cd "$(dirname "$OUTPUT_DOT_FILE")" && pwd)/$(basename "$OUTPUT_DOT_FILE")
cp "$TMP_DIR/graph.dot" "$ABS_OUTPUT"
echo "Successfully generated dependency graph at '${OUTPUT_DOT_FILE}'."
