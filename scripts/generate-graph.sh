#!/usr/bin/env bash
set -euo pipefail

# generate-graph.sh
# Generates a Terraform plan dependency graph (.dot) from a target configuration directory
# by creating an isolated temporary workspace with cloud and remote backend blocks safely replaced
# with a local backend, preventing configuration conflicts and avoiding remote network access.

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

# Resolve absolute output path before changing directories
ABS_OUTPUT_DIR=$(cd "$(dirname "$OUTPUT_DOT_FILE")" && pwd)
ABS_OUTPUT="${ABS_OUTPUT_DIR}/$(basename "$OUTPUT_DOT_FILE")"

# Determine workspace root to preserve parent-relative module sources (e.g. ../../modules)
# Use realpath to avoid symlink resolution mismatches (e.g. /var vs /private/var on macOS)
if git -C "$TARGET_DIR" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  REPO_ROOT=$(git -C "$TARGET_DIR" rev-parse --show-toplevel)
  COPY_ROOT="$REPO_ROOT"
  EXEC_SUBDIR=$(python3 -c "import os.path; print(os.path.relpath(os.path.realpath('$TARGET_DIR'), os.path.realpath('$REPO_ROOT')))")
else
  COPY_ROOT="$TARGET_DIR"
  EXEC_SUBDIR="."
fi

TMP_DIR=$(mktemp -d)
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

echo "Copying configuration to temporary workspace (dereferencing symlinks to preserve isolation)..."
# Use -h (--dereference) to copy file contents rather than symlinks, preventing unintended modification of originals
tar -C "$COPY_ROOT" -h --exclude='.git' --exclude='.terraform' -cf - . | tar -C "$TMP_DIR" -xf -

echo "Rewriting cloud and backend configurations to local backend in temporary copy..."
TMP_DIR="$TMP_DIR" python3 -c '
import os, json

backend_inserted = False

def find_closing_brace(content, start_pos):
    n = len(content)
    pos = start_pos
    depth = 1
    in_str = False
    in_line_comment = False
    in_block_comment = False
    while pos < n and depth > 0:
        c = content[pos]
        c2 = content[pos:pos+2]
        if in_line_comment:
            if c == "\n":
                in_line_comment = False
        elif in_block_comment:
            if c2 == "*/":
                in_block_comment = False
                pos += 1
        elif in_str:
            if c == "\\":
                pos += 1
            elif c == "\"":
                in_str = False
        else:
            if c2 == "//" or c == "#":
                in_line_comment = True
            elif c2 == "/*":
                in_block_comment = True
                pos += 1
            elif c == "\"":
                in_str = True
            elif c == "{":
                depth += 1
            elif c == "}":
                depth -= 1
                if depth == 0:
                    return pos
        pos += 1
    return pos if depth == 0 else -1

def clean_block_body(body):
    global backend_inserted
    bn = len(body)
    bpos = 0
    in_s = False
    in_lc = False
    in_bc = False
    res = []
    last_copied = 0
    changed = False

    while bpos < bn:
        c = body[bpos]
        c2 = body[bpos:bpos+2]
        if in_lc:
            if c == "\n":
                in_lc = False
        elif in_bc:
            if c2 == "*/":
                in_bc = False
                bpos += 1
        elif in_s:
            if c == "\\":
                bpos += 1
            elif c == "\"":
                in_s = False
        else:
            if c2 == "//" or c == "#":
                in_lc = True
            elif c2 == "/*":
                in_bc = True
                bpos += 1
            elif c == "\"":
                in_s = True
            else:
                target = None
                for kw in ["cloud", "backend"]:
                    if body[bpos:].startswith(kw):
                        before_ok = (bpos == 0 or not (body[bpos-1].isalnum() or body[bpos-1] == "_"))
                        after_pos = bpos + len(kw)
                        after_ok = (after_pos < bn and not (body[after_pos].isalnum() or body[after_pos] == "_"))
                        if before_ok and after_ok:
                            target = kw
                            break
                if target:
                    k = bpos + len(target)
                    in_lbl_str = False
                    found_brace = -1
                    while k < bn:
                        ch = body[k]
                        if ch == "\"":
                            in_lbl_str = not in_lbl_str
                        elif not in_lbl_str and ch == "{":
                            found_brace = k
                            break
                        elif not in_lbl_str and (ch == "\n" and target == "cloud"):
                            break
                        k += 1
                    if found_brace != -1:
                        depth = 1
                        ck = found_brace + 1
                        c_str = False
                        c_lc = False
                        c_bc = False
                        closing = -1
                        while ck < bn and depth > 0:
                            cc = body[ck]
                            cc2 = body[ck:ck+2]
                            if c_lc:
                                if cc == "\n": c_lc = False
                            elif c_bc:
                                if cc2 == "*/": c_bc = False; ck += 1
                            elif c_str:
                                if cc == "\\": ck += 1
                                elif cc == "\"": c_str = False
                            else:
                                if cc2 == "//" or cc == "#": c_lc = True
                                elif cc2 == "/*": c_bc = True; ck += 1
                                elif cc == "\"": c_str = True
                                elif cc == "{": depth += 1
                                elif cc == "}": depth -= 1; closing = ck
                            ck += 1
                        if depth == 0:
                            res.append(body[last_copied:bpos])
                            if not backend_inserted:
                                res.append("/* stripped " + target + " */\n  backend \"local\" {}\n")
                                backend_inserted = True
                            else:
                                res.append("/* stripped duplicate " + target + " */\n")
                            bpos = closing
                            last_copied = closing + 1
                            changed = True
        bpos += 1
    res.append(body[last_copied:])
    return "".join(res), changed

def process_hcl(content):
    n = len(content)
    pos = 0
    in_str = False
    in_line_comment = False
    in_block_comment = False
    out = []
    last_idx = 0

    while pos < n:
        c = content[pos]
        c2 = content[pos:pos+2]
        if in_line_comment:
            if c == "\n": in_line_comment = False
        elif in_block_comment:
            if c2 == "*/": in_block_comment = False; pos += 1
        elif in_str:
            if c == "\\": pos += 1
            elif c == "\"": in_str = False
        else:
            if c2 == "//" or c == "#": in_line_comment = True
            elif c2 == "/*": in_block_comment = True; pos += 1
            elif c == "\"": in_str = True
            elif content[pos:].startswith("terraform"):
                before_ok = (pos == 0 or not (content[pos-1].isalnum() or content[pos-1] == "_"))
                after_pos = pos + len("terraform")
                after_ok = (after_pos < n and not (content[after_pos].isalnum() or content[after_pos] == "_"))
                if before_ok and after_ok:
                    k = after_pos
                    while k < n and content[k].isspace(): k += 1
                    if k < n and content[k] == "{":
                        closing = find_closing_brace(content, k + 1)
                        if closing != -1:
                            body = content[k+1:closing]
                            cleaned_body, changed = clean_block_body(body)
                            if changed:
                                out.append(content[last_idx:k+1])
                                out.append(cleaned_body)
                                last_idx = closing
                                pos = closing
        pos += 1
    out.append(content[last_idx:])
    return "".join(out)

def process_tf_json(path):
    global backend_inserted
    try:
        with open(path, "r", encoding="utf-8") as f:
            data = json.load(f)
    except Exception:
        return
    if not isinstance(data, dict):
        return
    tf_configs = data.get("terraform")
    if tf_configs is None:
        return
    changed = False
    if isinstance(tf_configs, dict):
        if "cloud" in tf_configs:
            del tf_configs["cloud"]
            if not backend_inserted:
                tf_configs["backend"] = {"local": {}}
                backend_inserted = True
            changed = True
        elif "backend" in tf_configs:
            if not backend_inserted:
                tf_configs["backend"] = {"local": {}}
                backend_inserted = True
            else:
                del tf_configs["backend"]
            changed = True
    elif isinstance(tf_configs, list):
        for item in tf_configs:
            if isinstance(item, dict):
                if "cloud" in item:
                    del item["cloud"]
                    if not backend_inserted:
                        item["backend"] = {"local": {}}
                        backend_inserted = True
                    changed = True
                elif "backend" in item:
                    if not backend_inserted:
                        item["backend"] = {"local": {}}
                        backend_inserted = True
                    else:
                        del item["backend"]
                    changed = True
    if changed:
        with open(path, "w", encoding="utf-8") as f:
            json.dump(data, f, indent=2)

tmp_dir = os.environ.get("TMP_DIR", "")
if tmp_dir:
    for root, _, files in os.walk(tmp_dir):
        for file in files:
            path = os.path.join(root, file)
            if file.endswith(".tf"):
                try:
                    with open(path, "r", encoding="utf-8", errors="ignore") as f:
                        content = f.read()
                    new_content = process_hcl(content)
                    if new_content != content:
                        with open(path, "w", encoding="utf-8") as f:
                            f.write(new_content)
                except Exception:
                    pass
            elif file.endswith(".tf.json"):
                process_tf_json(path)
'

echo "Initializing Terraform in isolated workspace (subdirectory: ${EXEC_SUBDIR})..."
(
  cd "${TMP_DIR}/${EXEC_SUBDIR}"
  terraform init -input=false
  echo "Generating plan dependency graph with -type=plan..."
  terraform graph -type=plan > graph.dot
)

cp "${TMP_DIR}/${EXEC_SUBDIR}/graph.dot" "$ABS_OUTPUT"
echo "Successfully generated dependency graph at '${OUTPUT_DOT_FILE}'."
