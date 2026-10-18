#!/usr/bin/env bash
set -euo pipefail

# fetch-tfc-log.sh
# Downloads the plan/apply log from HCP Terraform (Terraform Cloud) via REST API.

TFC_TOKEN="${1:-}"
TFC_ORG="${2:-}"
TFC_WORKSPACE="${3:-}"
TARGET_RUN_ID="${4:-}"
OUTPUT_LOG_FILE="${5:-tfc_execution.log}"
TARGET_COMMIT_SHA="${6:-}"

if [[ -z "$TFC_TOKEN" || -z "$TFC_ORG" || -z "$TFC_WORKSPACE" ]]; then
  echo "::error::TFC_TOKEN, TFC_ORG, and TFC_WORKSPACE must be provided."
  exit 1
fi

TFC_API_BASE="https://app.terraform.io/api/v2"

echo "Fetching Workspace ID for '${TFC_ORG}/${TFC_WORKSPACE}'..."
WORKSPACE_RESP=$(curl -s -f -H "Authorization: Bearer ${TFC_TOKEN}" \
  -H "Content-Type: application/vnd.api+json" \
  "${TFC_API_BASE}/organizations/${TFC_ORG}/workspaces/${TFC_WORKSPACE}")

WORKSPACE_ID=$(echo "$WORKSPACE_RESP" | jq -r '.data.id')
if [[ -z "$WORKSPACE_ID" || "$WORKSPACE_ID" == "null" ]]; then
  echo "::error::Could not find workspace '${TFC_WORKSPACE}' in organization '${TFC_ORG}'."
  exit 1
fi

echo "Found Workspace ID: ${WORKSPACE_ID}"

RUN_ID="$TARGET_RUN_ID"
if [[ -z "$RUN_ID" ]]; then
  echo "Searching for runs in workspace..."
  RUNS_RESP=$(curl -s -f -H "Authorization: Bearer ${TFC_TOKEN}" \
    -H "Content-Type: application/vnd.api+json" \
    "${TFC_API_BASE}/workspaces/${WORKSPACE_ID}/runs?page%5Bsize%5D=10")

  if [[ -n "$TARGET_COMMIT_SHA" ]]; then
    echo "Matching run with commit SHA: ${TARGET_COMMIT_SHA}..."
    RUN_ID=$(echo "$RUNS_RESP" | jq -r --arg sha "$TARGET_COMMIT_SHA" \
      '.data[] | select(.attributes."commit-sha" == $sha or (.attributes.message // "" | contains($sha))) | .id' | head -n 1)
  fi

  if [[ -z "$RUN_ID" || "$RUN_ID" == "null" ]]; then
    echo "Commit-specific run not found, falling back to the latest run in workspace..."
    RUN_ID=$(echo "$RUNS_RESP" | jq -r '.data[0].id')
  fi
fi

if [[ -z "$RUN_ID" || "$RUN_ID" == "null" ]]; then
  echo "::error::No run found in workspace '${TFC_WORKSPACE}'."
  exit 1
fi

echo "Using Run ID: ${RUN_ID}"

# Poll run until plan/apply log is available
echo "Checking Run details..."
RUN_DETAIL=$(curl -s -f -H "Authorization: Bearer ${TFC_TOKEN}" \
  -H "Content-Type: application/vnd.api+json" \
  "${TFC_API_BASE}/runs/${RUN_ID}")

PLAN_ID=$(echo "$RUN_DETAIL" | jq -r '.data.relationships.plan.data.id // empty')
APPLY_ID=$(echo "$RUN_DETAIL" | jq -r '.data.relationships.apply.data.id // empty')
STATUS=$(echo "$RUN_DETAIL" | jq -r '.data.attributes.status')

echo "Run Status: ${STATUS}"

# Prefer apply log if completed or applying, otherwise fetch plan log
LOG_ENDPOINT=""
if [[ -n "$APPLY_ID" && "$APPLY_ID" != "null" ]]; then
  echo "Apply phase found (ID: ${APPLY_ID}). Fetching apply log..."
  LOG_ENDPOINT="${TFC_API_BASE}/applies/${APPLY_ID}/log"
elif [[ -n "$PLAN_ID" && "$PLAN_ID" != "null" ]]; then
  echo "Plan phase found (ID: ${PLAN_ID}). Fetching plan log..."
  LOG_ENDPOINT="${TFC_API_BASE}/plans/${PLAN_ID}/log"
else
  echo "::error::Neither Plan nor Apply ID found for Run ${RUN_ID}."
  exit 1
fi

curl -s -f -L -H "Authorization: Bearer ${TFC_TOKEN}" \
  "${LOG_ENDPOINT}" > "${OUTPUT_LOG_FILE}"

LOG_SIZE=$(wc -c < "${OUTPUT_LOG_FILE}" | tr -d ' ')
echo "Successfully downloaded log to ${OUTPUT_LOG_FILE} (${LOG_SIZE} bytes)."
