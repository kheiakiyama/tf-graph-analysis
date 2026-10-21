#!/usr/bin/env bash
set -euo pipefail

# trigger-tfc-run.sh
# Triggers an ad-hoc speculative plan-only run on an HCP Terraform (Terraform Cloud) workspace via API.

TFC_TOKEN="${1:-}"
TFC_ORG="${2:-}"
TFC_WORKSPACE="${3:-}"
RUN_MESSAGE="${4:-Scheduled critical path analysis run triggered by GitHub Actions}"

if [[ -z "$TFC_TOKEN" || -z "$TFC_ORG" || -z "$TFC_WORKSPACE" ]]; then
  echo "::error::TFC_TOKEN, TFC_ORG, and TFC_WORKSPACE must be provided."
  exit 1
fi

TFC_API_BASE="https://app.terraform.io/api/v2"

echo "Resolving Workspace ID for '${TFC_ORG}/${TFC_WORKSPACE}'..."
WS_RESP=$(curl -s -f -H "Authorization: Bearer ${TFC_TOKEN}" \
  -H "Content-Type: application/vnd.api+json" \
  "${TFC_API_BASE}/organizations/${TFC_ORG}/workspaces/${TFC_WORKSPACE}")

WORKSPACE_ID=$(echo "$WS_RESP" | jq -r '.data.id // empty')
if [[ -z "$WORKSPACE_ID" || "$WORKSPACE_ID" == "null" ]]; then
  echo "::error::Could not find workspace '${TFC_WORKSPACE}' in organization '${TFC_ORG}'."
  exit 1
fi

echo "Queueing plan-only run for workspace ${WORKSPACE_ID}..."
PAYLOAD=$(jq -n \
  --arg ws_id "$WORKSPACE_ID" \
  --arg msg "$RUN_MESSAGE" \
  '{
    data: {
      type: "runs",
      attributes: {
        "is-destroy": false,
        "plan-only": true,
        message: $msg
      },
      relationships: {
        workspace: {
          data: {
            type: "workspaces",
            id: $ws_id
          }
        }
      }
    }
  }')

RUN_RESP=$(curl -s -f -X POST \
  -H "Authorization: Bearer ${TFC_TOKEN}" \
  -H "Content-Type: application/vnd.api+json" \
  -d "$PAYLOAD" \
  "${TFC_API_BASE}/runs")

RUN_ID=$(echo "$RUN_RESP" | jq -r '.data.id // empty')
if [[ -z "$RUN_ID" || "$RUN_ID" == "null" ]]; then
  echo "::error::Failed to queue run in workspace '${TFC_WORKSPACE}'."
  exit 1
fi

echo "Successfully queued HCP Terraform run: ${RUN_ID} (plan-only)"

if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
  echo "run_id=${RUN_ID}" >> "$GITHUB_OUTPUT"
  echo "workspace_id=${WORKSPACE_ID}" >> "$GITHUB_OUTPUT"
fi
