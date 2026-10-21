#!/usr/bin/env bash
set -euo pipefail

# manage-tfc-var.sh
# Manages the temporary lifecycle of TF_LOG_CORE environment variable on an HCP Terraform workspace.
# Supports 'setup' (captures original state and sets TRACE) and 'restore' (restores or deletes).

COMMAND="${1:-}"
TFC_TOKEN="${2:-}"
TFC_API_BASE="https://app.terraform.io/api/v2"

if [[ -z "$COMMAND" || -z "$TFC_TOKEN" ]]; then
  echo "::error::Command ('setup' or 'restore') and TFC_TOKEN are required."
  exit 1
fi

case "$COMMAND" in
  setup)
    TFC_ORG="${3:-}"
    TFC_WORKSPACE="${4:-}"
    if [[ -z "$TFC_ORG" || -z "$TFC_WORKSPACE" ]]; then
      echo "::error::TFC_ORG and TFC_WORKSPACE are required for 'setup'."
      exit 1
    fi

    echo "Resolving Workspace ID for '${TFC_ORG}/${TFC_WORKSPACE}'..."
    WS_RESP=$(curl -s -f -H "Authorization: Bearer ${TFC_TOKEN}" \
      -H "Content-Type: application/vnd.api+json" \
      "${TFC_API_BASE}/organizations/${TFC_ORG}/workspaces/${TFC_WORKSPACE}")

    WORKSPACE_ID=$(echo "$WS_RESP" | jq -r '.data.id // empty')
    if [[ -z "$WORKSPACE_ID" || "$WORKSPACE_ID" == "null" ]]; then
      echo "::error::Could not find workspace '${TFC_WORKSPACE}' in organization '${TFC_ORG}'."
      exit 1
    fi

    echo "Fetching current variables for workspace ${WORKSPACE_ID}..."
    VARS_RESP=$(curl -s -f -H "Authorization: Bearer ${TFC_TOKEN}" \
      -H "Content-Type: application/vnd.api+json" \
      "${TFC_API_BASE}/workspaces/${WORKSPACE_ID}/vars")

    EXISTING_VAR=$(echo "$VARS_RESP" | jq -r '
      .data[]? | select(.attributes.key == "TF_LOG_CORE" and .attributes.category == "env") |
      {id: .id, value: .attributes.value}
    ')

    if [[ -n "$EXISTING_VAR" && "$EXISTING_VAR" != "null" ]]; then
      ORIG_ID=$(echo "$EXISTING_VAR" | jq -r '.id')
      ORIG_VAL=$(echo "$EXISTING_VAR" | jq -r '.value // empty')
      echo "Found existing TF_LOG_CORE variable (ID: ${ORIG_ID}, value: '${ORIG_VAL}')."
      echo "Temporarily updating TF_LOG_CORE to 'TRACE'..."

      UPDATE_PAYLOAD=$(jq -n \
        --arg id "$ORIG_ID" \
        --arg val "TRACE" \
        '{
          data: {
            id: $id,
            type: "vars",
            attributes: {
              value: $val
            }
          }
        }')

      curl -s -f -X PATCH \
        -H "Authorization: Bearer ${TFC_TOKEN}" \
        -H "Content-Type: application/vnd.api+json" \
        -d "$UPDATE_PAYLOAD" \
        "${TFC_API_BASE}/workspaces/${WORKSPACE_ID}/vars/${ORIG_ID}" > /dev/null

      if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
        echo "workspace_id=${WORKSPACE_ID}" >> "$GITHUB_OUTPUT"
        echo "var_existed=true" >> "$GITHUB_OUTPUT"
        echo "orig_var_id=${ORIG_ID}" >> "$GITHUB_OUTPUT"
        echo "orig_var_val=${ORIG_VAL}" >> "$GITHUB_OUTPUT"
        echo "created_var_id=" >> "$GITHUB_OUTPUT"
      fi
    else
      echo "No existing TF_LOG_CORE variable found on workspace. Creating temporary TF_LOG_CORE=TRACE..."
      CREATE_PAYLOAD=$(jq -n \
        '{
          data: {
            type: "vars",
            attributes: {
              key: "TF_LOG_CORE",
              value: "TRACE",
              category: "env",
              hcl: false,
              sensitive: false
            }
          }
        }')

      CREATE_RESP=$(curl -s -f -X POST \
        -H "Authorization: Bearer ${TFC_TOKEN}" \
        -H "Content-Type: application/vnd.api+json" \
        -d "$CREATE_PAYLOAD" \
        "${TFC_API_BASE}/workspaces/${WORKSPACE_ID}/vars")

      CREATED_ID=$(echo "$CREATE_RESP" | jq -r '.data.id // empty')
      if [[ -z "$CREATED_ID" || "$CREATED_ID" == "null" ]]; then
        echo "::error::Failed to create temporary TF_LOG_CORE variable on workspace."
        exit 1
      fi
      echo "Created temporary TF_LOG_CORE variable (ID: ${CREATED_ID})."

      if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
        echo "workspace_id=${WORKSPACE_ID}" >> "$GITHUB_OUTPUT"
        echo "var_existed=false" >> "$GITHUB_OUTPUT"
        echo "orig_var_id=" >> "$GITHUB_OUTPUT"
        echo "orig_var_val=" >> "$GITHUB_OUTPUT"
        echo "created_var_id=${CREATED_ID}" >> "$GITHUB_OUTPUT"
      fi
    fi
    echo "TF_LOG_CORE=TRACE configured successfully for workspace ${WORKSPACE_ID}."
    ;;

  restore)
    WORKSPACE_ID="${3:-}"
    VAR_EXISTED="${4:-false}"
    ORIG_VAR_ID="${5:-}"
    ORIG_VAR_VAL="${6:-}"
    CREATED_VAR_ID="${7:-}"

    if [[ -z "$WORKSPACE_ID" ]]; then
      echo "::warning::Workspace ID missing in restore step. Skipping variable cleanup."
      exit 0
    fi

    echo "Restoring TF_LOG_CORE configuration on workspace ${WORKSPACE_ID}..."

    if [[ "$VAR_EXISTED" == "true" ]]; then
      if [[ -n "$ORIG_VAR_ID" ]]; then
        echo "Reverting TF_LOG_CORE (ID: ${ORIG_VAR_ID}) to original value: '${ORIG_VAR_VAL}'..."
        RESTORE_PAYLOAD=$(jq -n \
          --arg id "$ORIG_VAR_ID" \
          --arg val "$ORIG_VAR_VAL" \
          '{
            data: {
              id: $id,
              type: "vars",
              attributes: {
                value: $val
              }
            }
          }')

        if curl -s -f -X PATCH \
          -H "Authorization: Bearer ${TFC_TOKEN}" \
          -H "Content-Type: application/vnd.api+json" \
          -d "$RESTORE_PAYLOAD" \
          "${TFC_API_BASE}/workspaces/${WORKSPACE_ID}/vars/${ORIG_VAR_ID}" > /dev/null; then
          echo "Successfully restored original TF_LOG_CORE value."
        else
          echo "::warning::Failed to restore original TF_LOG_CORE value (ID: ${ORIG_VAR_ID})."
        fi
      fi
    else
      if [[ -n "$CREATED_VAR_ID" ]]; then
        echo "Deleting temporary TF_LOG_CORE variable (ID: ${CREATED_VAR_ID})..."
        if curl -s -f -X DELETE \
          -H "Authorization: Bearer ${TFC_TOKEN}" \
          -H "Content-Type: application/vnd.api+json" \
          "${TFC_API_BASE}/workspaces/${WORKSPACE_ID}/vars/${CREATED_VAR_ID}" > /dev/null; then
          echo "Successfully deleted temporary TF_LOG_CORE variable."
        else
          echo "::warning::Failed to delete temporary TF_LOG_CORE variable (ID: ${CREATED_VAR_ID})."
        fi
      fi
    fi
    ;;

  *)
    echo "::error::Unknown command '$COMMAND'. Expected 'setup' or 'restore'."
    exit 1
    ;;
esac
