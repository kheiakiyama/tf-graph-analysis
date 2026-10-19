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
  FIND_RUN_TIMEOUT=60
  FIND_START=$(date +%s)

  while [[ -z "$RUN_ID" || "$RUN_ID" == "null" ]]; do
    RUNS_RESP=$(curl -s -f -H "Authorization: Bearer ${TFC_TOKEN}" \
      -H "Content-Type: application/vnd.api+json" \
      "${TFC_API_BASE}/workspaces/${WORKSPACE_ID}/runs?include=configuration_version.ingress_attributes&page%5Bsize%5D=10")

    if [[ -n "$TARGET_COMMIT_SHA" ]]; then
      echo "Matching run with commit SHA: ${TARGET_COMMIT_SHA}..."
      RUN_ID=$(echo "$RUNS_RESP" | jq -r --arg sha "$TARGET_COMMIT_SHA" '
        . as $root |
        [
          $root.data[]? | select(
            .attributes["commit-sha"] == $sha or
            ((.attributes.message // "") | contains($sha)) or
            (
              .relationships["configuration-version"].data.id as $cv_id |
              ($root.included[]? | select(.type == "configuration-versions" and .id == $cv_id) |
                (.attributes["commit-sha"] == $sha or
                 (
                   .relationships["ingress-attributes"].data.id as $ia_id |
                   ($root.included[]? | select(.type == "ingress-attributes" and .id == $ia_id) | .attributes["commit-sha"] == $sha)
                 )
                )
              )
            )
          ) | .id
        ][0] // empty
      ')
    else
      # If no commit SHA is provided, use the latest run
      RUN_ID=$(echo "$RUNS_RESP" | jq -r '.data[0].id // empty')
      break
    fi

    if [[ -n "$RUN_ID" && "$RUN_ID" != "null" ]]; then
      break
    fi

    NOW=$(date +%s)
    if (( NOW - FIND_START >= FIND_RUN_TIMEOUT )); then
      echo "::error::No run matching commit SHA '${TARGET_COMMIT_SHA}' found in workspace '${TFC_WORKSPACE}' after ${FIND_RUN_TIMEOUT}s."
      exit 1
    fi

    echo "Run for commit ${TARGET_COMMIT_SHA} not detected yet. Waiting for HCP Terraform to trigger run..."
    sleep 5
  done
fi

if [[ -z "$RUN_ID" || "$RUN_ID" == "null" ]]; then
  echo "::error::No run found in workspace '${TFC_WORKSPACE}'."
  exit 1
fi

echo "Using Run ID: ${RUN_ID}"

# Poll run until plan (or apply) phase finishes
POLL_INTERVAL=5
POLL_TIMEOUT="${TFC_POLL_TIMEOUT:-900}" # Default to 15 minutes
START_TIME=$(date +%s)

echo "Waiting for Run ${RUN_ID} plan/apply to complete (timeout: ${POLL_TIMEOUT}s)..."

PLAN_ID=""
APPLY_ID=""
STATUS=""

while true; do
  CURRENT_TIME=$(date +%s)
  ELAPSED=$(( CURRENT_TIME - START_TIME ))

  if (( ELAPSED >= POLL_TIMEOUT )); then
    echo "::error::Timed out after ${ELAPSED}s waiting for Run ${RUN_ID} to complete (last status: ${STATUS})."
    exit 1
  fi

  RUN_DETAIL=$(curl -s -f -H "Authorization: Bearer ${TFC_TOKEN}" \
    -H "Content-Type: application/vnd.api+json" \
    "${TFC_API_BASE}/runs/${RUN_ID}")

  STATUS=$(echo "$RUN_DETAIL" | jq -r '.data.attributes.status // empty')
  PLAN_ID=$(echo "$RUN_DETAIL" | jq -r '.data.relationships.plan.data.id // empty')
  APPLY_ID=$(echo "$RUN_DETAIL" | jq -r '.data.relationships.apply.data.id // empty')

  echo "Run status: ${STATUS} (elapsed: ${ELAPSED}s)"

  case "$STATUS" in
    # Terminal / completed plan statuses where logs are fully available
    planned|planned_and_finished|needs_confirmation|cost_estimated|policy_checked|policy_override|policy_soft_failed|confirmed|applied|errored|plan_errored|canceled|force_canceled|discarded)
      echo "Run reached terminal/ready state '${STATUS}'. Fetching log..."
      break
      ;;
    # In-progress statuses
    pending|fetching|fetching_completed|pre_plan_running|pre_plan_completed|queuing|plan_queued|planning|cost_estimating|policy_checking|apply_queued|applying|*)
      sleep "$POLL_INTERVAL"
      ;;
  esac
done

# Fetch log-read-url from apply (if executed) or plan resource
LOG_READ_URL=""
if [[ -n "$APPLY_ID" && "$APPLY_ID" != "null" && ("$STATUS" == "applied" || "$STATUS" == "errored" || "$STATUS" == "canceled" || "$STATUS" == "force_canceled") ]]; then
  echo "Apply phase found (ID: ${APPLY_ID}). Fetching apply details for log-read-url..."
  APPLY_RESP=$(curl -s -f -H "Authorization: Bearer ${TFC_TOKEN}" \
    -H "Content-Type: application/vnd.api+json" \
    "${TFC_API_BASE}/applies/${APPLY_ID}")
  LOG_READ_URL=$(echo "$APPLY_RESP" | jq -r '.data.attributes["log-read-url"] // empty')
fi

if [[ -z "$LOG_READ_URL" && -n "$PLAN_ID" && "$PLAN_ID" != "null" ]]; then
  echo "Plan phase found (ID: ${PLAN_ID}). Fetching plan details for log-read-url..."
  PLAN_RESP=$(curl -s -f -H "Authorization: Bearer ${TFC_TOKEN}" \
    -H "Content-Type: application/vnd.api+json" \
    "${TFC_API_BASE}/plans/${PLAN_ID}")
  LOG_READ_URL=$(echo "$PLAN_RESP" | jq -r '.data.attributes["log-read-url"] // empty')
fi

if [[ -z "$LOG_READ_URL" ]]; then
  echo "::error::Could not resolve 'log-read-url' for Plan (${PLAN_ID}) or Apply (${APPLY_ID}) in Run ${RUN_ID}."
  exit 1
fi

echo "Downloading execution log via log-read-url..."
# Note: log-read-url is a pre-signed storage URL (e.g. archivist.terraform.io).
# Do NOT send the Authorization: Bearer header to this URL, as object storage rejects it with 400 Bad Request.
curl -s -f -L "${LOG_READ_URL}" > "${OUTPUT_LOG_FILE}"

LOG_SIZE=$(wc -c < "${OUTPUT_LOG_FILE}" | tr -d ' ')
echo "Successfully downloaded log to ${OUTPUT_LOG_FILE} (${LOG_SIZE} bytes)."

if [[ "$LOG_SIZE" -eq 0 ]]; then
  echo "::warning::Downloaded log file is empty. Please verify that the run output contains trace logs."
fi
