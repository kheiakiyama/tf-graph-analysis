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
import os, re, json

backend_inserted = False

def skip_comments_and_spaces(content, p):
    n = len(content)
    while p < n:
        if content[p].isspace():
            p += 1
        elif content[p:p+2] == "//" or content[p] == "#":
            idx = content.find("\n", p)
            p = n if idx == -1 else idx + 1
        elif content[p:p+2] == "/*":
            idx = content.find("*/", p + 2)
            p = n if idx == -1 else idx + 2
        else:
            break
    return p

def check_heredoc(content, pos):
    n = len(content)
    if content[pos:pos+2] == "<<" and (pos+2 < n and (content[pos+2].isalpha() or content[pos+2] == "-")):
        m = re.match(r"<<-?([A-Za-z_][A-Za-z0-9_]*)", content[pos:])
        if m:
            marker = m.group(1)
            is_ind = (content[pos+2] == "-")
            nl = content.find("\n", pos)
            if nl != -1:
                pat = re.compile(r"^[ \t]*" + re.escape(marker) + r"\s*$", re.MULTILINE) if is_ind else re.compile(r"^" + re.escape(marker) + r"\s*$", re.MULTILINE)
                m2 = pat.search(content, nl + 1)
                if m2:
                    return m2.end()
    return None

def find_closing_brace(content, start_pos):
    n = len(content)
    pos = start_pos
    depth = 1
    in_str = False
    in_lc = False
    in_bc = False
    while pos < n and depth > 0:
        c = content[pos]
        c2 = content[pos:pos+2]
        if in_lc:
            if c == "\n": in_lc = False
        elif in_bc:
            if c2 == "*/": in_bc = False; pos += 1
        elif in_str:
            if c == "\\": pos += 1
            elif c == "\"": in_str = False
        else:
            if c2 == "//" or c == "#": in_lc = True
            elif c2 == "/*": in_bc = True; pos += 1
            elif c == "\"": in_str = True
            else:
                h_end = check_heredoc(content, pos)
                if h_end:
                    pos = h_end - 1
                elif c == "{": depth += 1
                elif c == "}":
                    depth -= 1
                    if depth == 0: return pos
        pos += 1
    return pos if depth == 0 else -1

def clean_block_body(body):
    global backend_inserted
    bn = len(body)
    bpos = 0
    res = []
    last_copied = 0
    depth = 0
    in_str = False
    in_lc = False
    in_bc = False
    changed = False

    while bpos < bn:
        c = body[bpos]
        c2 = body[bpos:bpos+2]
        if in_lc:
            if c == "\n": in_lc = False
        elif in_bc:
            if c2 == "*/": in_bc = False; bpos += 1
        elif in_str:
            if c == "\\": bpos += 1
            elif c == "\"": in_str = False
        else:
            if c2 == "//" or c == "#": in_lc = True
            elif c2 == "/*": in_bc = True; bpos += 1
            elif c == "\"": in_str = True
            else:
                h_end = check_heredoc(body, bpos)
                if h_end:
                    bpos = h_end - 1
                elif c == "{": depth += 1
                elif c == "}": depth -= 1
                elif depth == 0:
                    # Only match cloud { ... } or backend ... { ... } at depth == 0 of terraform block
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
                        p = skip_comments_and_spaces(body, bpos + len(target))
                        found_brace = -1
                        if target == "cloud":
                            if p < bn and body[p] == "{":
                                found_brace = p
                        elif target == "backend":
                            # Backend has a label: quoted string or identifier (never an '=' sign)
                            if p < bn and body[p] == "\"":
                                end_lbl = p + 1
                                while end_lbl < bn:
                                    if body[end_lbl] == "\\": end_lbl += 2
                                    elif body[end_lbl] == "\"": break
                                    else: end_lbl += 1
                                if end_lbl < bn and body[end_lbl] == "\"":
                                    p = skip_comments_and_spaces(body, end_lbl + 1)
                            else:
                                m_id = re.match(r"[A-Za-z0-9_]+", body[p:])
                                if m_id:
                                    p = skip_comments_and_spaces(body, p + len(m_id.group(0)))
                            if p < bn and body[p] == "{":
                                found_brace = p

                        if found_brace != -1:
                            closing_pos = find_closing_brace(body, found_brace + 1)
                            if closing_pos != -1:
                                res.append(body[last_copied:bpos])
                                if not backend_inserted:
                                    res.append("/* stripped " + target + " */\n  backend \"local\" {}\n")
                                    backend_inserted = True
                                else:
                                    res.append("/* stripped duplicate " + target + " */\n")
                                bpos = closing_pos
                                last_copied = closing_pos + 1
                                changed = True
        bpos += 1
    res.append(body[last_copied:])
    return "".join(res), changed

def process_hcl(content):
    n = len(content)
    pos = 0
    in_str = False
    in_lc = False
    in_bc = False
    out = []
    last_idx = 0

    while pos < n:
        c = content[pos]
        c2 = content[pos:pos+2]
        if in_lc:
            if c == "\n": in_lc = False
        elif in_bc:
            if c2 == "*/": in_bc = False; pos += 1
        elif in_str:
            if c == "\\": pos += 1
            elif c == "\"": in_str = False
        else:
            if c2 == "//" or c == "#": in_lc = True
            elif c2 == "/*": in_bc = True; pos += 1
            elif c == "\"": in_str = True
            else:
                h_end = check_heredoc(content, pos)
                if h_end:
                    pos = h_end - 1
                elif content[pos:].startswith("terraform"):
                    before_ok = (pos == 0 or not (content[pos-1].isalnum() or content[pos-1] == "_"))
                    after_pos = pos + len("terraform")
                    after_ok = (after_pos < n and not (content[after_pos].isalnum() or content[after_pos] == "_"))
                    if before_ok and after_ok:
                        k = skip_comments_and_spaces(content, after_pos)
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
