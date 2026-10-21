# tf-graph-analysis

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go)](https://go.dev)

> 🇯🇵 [日本語ドキュメント (Japanese)](README.ja.md)

**tf-graph-analysis** is a performance profiling CLI tool for Terraform. It combines the static dependency graph (DAG) from `terraform graph` with actual execution timings from `TF_LOG=TRACE`, using **Critical Path Method (CPM)** to identify and visualize the exact sequential bottleneck chains causing slow plans and applies.

---

## Motivation

As infrastructure code grows, Terraform execution times often become excruciatingly slow.

Because Terraform executes independent resources in parallel and dependent resources sequentially, human operators often struggle to distinguish between:
1. **"Resources that are individually slow"** (but run in parallel without blocking others)
2. **"Sequential dependency chains"** (the true critical path that directly dictates the total runtime)

Optimizing a slow resource that has idle slack time yields **zero improvement** in overall execution time. 

**tf-graph-analysis** maps real-world execution durations onto the Terraform DAG, extracts the true critical path, calculates slack for every resource, and gives you data-driven insights on where to split state files or untangle implicit dependencies.

---

## Features

- **Critical Path Analysis (CPM)**: Automatically detects the longest sequential bottleneck chain using topological sort and dynamic programming ($O(V+E)$).
- **Float / Slack Calculation**: Quantifies how much each resource can be delayed before impacting overall execution time.
- **Multiple Visualization Formats**:
  - 🖥️ **Console Summary**: Clean terminal table of the bottleneck chain and slowest resource rankings.
  - 🌐 **Standalone Interactive HTML (`-output-html`)**:
    - **100% Local & Zero External Network Requests**: Safe for confidential infrastructure data.
    - Interactive waterfall timeline (Gantt chart), resource search, and 0s instant-node toggle.
  - ⏱️ **Chrome Trace Event Format (`-output-trace`)**:
    - Open with `chrome://tracing` or [ui.perfetto.dev](https://ui.perfetto.dev) for drag-and-drop timeline inspection.
  - 🎨 **Highlighted Graphviz DOT (`-output-dot`)**:
    - Generates DOT files with critical paths bolded in red for rendering with `dot -Tsvg`.

---

## Quick Start

### Installation

#### Download Pre-built Binary
Download the latest binary for your operating system (Linux, macOS, Windows) from the [Releases](https://github.com/kheiakiyama/tf-graph-analysis/releases) page.

> [!NOTE]
> **macOS Note**: Downloaded binaries may be quarantined by macOS Gatekeeper. If blocked, remove the quarantine attribute:
> ```bash
> xattr -d com.apple.quarantine ./tf-graph-analysis
> ```

#### Install via Go
```bash
go install github.com/kheiakiyama/tf-graph-analysis/cmd/tf-graph-analysis@latest
```

#### Build from Source
```bash
git clone https://github.com/kheiakiyama/tf-graph-analysis.git
cd tf-graph-analysis
go build -o tf-graph-analysis ./cmd/tf-graph-analysis
```

---

### Step 1: Collect Terraform Artifacts

You need two files from your Terraform configuration:

```bash
# 1. Export the dependency graph (DOT format)
terraform graph -type=plan > graph.dot
```

```bash
# 2. Export execution logs with detailed timings
```

#### How to Collect Logs:

##### A. Local Execution
Run Terraform with `TF_LOG=TRACE`:
```bash
TF_LOG=TRACE terraform plan -no-color > tf_plan.log 2>&1
```

##### B. HCP Terraform / Terraform Cloud / Enterprise (Remote Execution)
> [!IMPORTANT]
> When using `backend "cloud"`, setting `TF_LOG=TRACE` locally only affects your local CLI and does NOT pass down to the remote worker container.
> 
> To capture full execution traces on Terraform Cloud:
> 1. Go to your **Workspace Settings** > **Variables** in the HCP Terraform console.
> 2. Add an **Environment variable**:
>    - **Key**: `TF_LOG_CORE`
>    - **Value**: `TRACE`
> 3. Run `terraform plan` and download or save the complete output log to `tf_plan.log`.

---

### Step 2: Run Analysis

```bash
# Basic console analysis
./tf-graph-analysis -dot graph.dot -log tf_plan.log

# Export interactive HTML report and Chrome Trace JSON
./tf-graph-analysis \
  -dot graph.dot \
  -log tf_plan.log \
  -output-html report.html \
  -output-trace trace.json \
  -output-dot highlighted.dot

# Open the HTML report locally in your browser
open report.html
```

---

## GitHub Action CI & Scheduled Integration

`tf-graph-analysis` can be integrated into your GitHub Actions workflows. Depending on your infrastructure architecture and Terraform execution mode, there are **two distinct usage patterns**:

| Pattern | Execution Environment | Intended Triggers | Key Characteristics |
|:---|:---|:---|:---|
| **Pattern A: Local Execution** | GitHub Actions Runner | **CI (`pull_request`), Cron (`schedule`), Manual** | Runs `terraform plan` on the GHA runner. TRACE logging is confined locally to the runner, making it ideal for automatic PR checks and scheduled benchmarks without affecting any web UI. |
| **Pattern B: Remote Execution** | HCP Terraform (TFC) | **Cron (`schedule`) only, Manual** | Fetches remotely executed plan logs via the TFC API. **Scheduled execution is strongly recommended to prevent degrading everyday TFC Web GUI responsiveness.** |

---

### Pattern A: Running Terraform directly in GitHub Actions

When running `terraform plan` on the GitHub Actions runner, TRACE logs are written only to a local file on the runner VM, keeping external web UIs completely untouched. It is safe, fast, and suitable for every Pull Request and scheduled runs.

```yaml
name: Terraform Plan & Critical Path Analysis

on:
  pull_request:
    branches: [ main ]
  schedule:
    # Every Monday at 02:00 JST (17:00 UTC Sunday)
    - cron: '0 17 * * 0'
  workflow_dispatch:

jobs:
  plan:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      pull-requests: write # Required for PR comments (PR events only)
    steps:
      - uses: actions/checkout@v7
      - uses: hashicorp/setup-terraform@v3

      - name: Terraform Init & Plan
        run: |
          terraform init
          terraform graph -type=plan > graph.dot
          TF_LOG=TRACE terraform plan -no-color > tf_plan.log 2>&1 || true

      - name: Critical Path Analysis
        uses: kheiakiyama/tf-graph-analysis@v0.1.7
        with:
          dot-file: 'graph.dot'
          log-file: 'tf_plan.log'
          pr-comment: 'true'

> 💡 **Report Destinations**:
> - **PR Comment**: Automatically posted/updated when triggered by a Pull Request.
> - **GitHub Actions Summary**: Rendered directly in the Actions run Summary tab.
> - **HTML Report**: Full interactive waterfall chart is saved as a workflow artifact (`terraform-critical-path-report`).
```

---

### Pattern B: Using HCP Terraform (Terraform Cloud) Remote Execution

When using HCP Terraform Remote Execution, the plan runs remotely on HCP Terraform worker VMs.

#### ⚠️ Critical Considerations for TFC (Why Scheduled Execution is Recommended)

1. **Need for `TF_LOG_CORE: TRACE`**:
   - Unlike `terraform apply`, `terraform plan` does not create or modify resources; it only refreshes state. Standard logs do not output individual resource timings.
   - Accurately measuring resource-level execution duration requires `TF_LOG_CORE=TRACE`.
2. **Impact on TFC Web GUI**:
   - HCP Terraform streams all standard output directly into the browser GUI.
   - For workspaces with hundreds to 1,000+ resources, enabling `TF_LOG_CORE=TRACE` produces tens of thousands of log lines, causing the **TFC Web GUI to freeze or lag significantly**, degrading day-to-day developer review and approval workflows.
   - Therefore, **we strongly recommend running this via Cron (e.g. nightly or weekly) rather than on every PR**.

#### Scheduled Workflow Configuration Example

This Action automates the temporary lifecycle of the required log level:
- `tfc-manage-log-level: 'true'`: Captures the existing state of workspace variables, temporarily sets `TF_LOG_CORE=TRACE`, and **guarantees restoration (or deletion) of the original state upon completion (even if failures occur)** via an `if: always()` step.
- `tfc-trigger-run: 'true'`: Automatically queues a safe, speculative `plan-only` run via the TFC API without waiting for VCS webhooks.

```yaml
name: Scheduled Terraform Critical Path Analysis (TFC)

on:
  schedule:
    # Every Monday at 02:00 JST (17:00 UTC Sunday)
    - cron: '0 17 * * 0'
  workflow_dispatch: # Allows manual on-demand execution

jobs:
  analyze:
    runs-on: ubuntu-latest
    permissions:
      contents: read
    steps:
      - uses: actions/checkout@v7

      - name: Critical Path Analysis (TFC Scheduled)
        uses: kheiakiyama/tf-graph-analysis@v0.1.7
        with:
          tfc-token: ${{ secrets.TFC_TOKEN }}
          tfc-organization: 'my-org'
          tfc-workspace: 'my-workspace'
          working-directory: 'terraform/envs/prod'
          tfc-trigger-run: 'true'          # Queues a speculative plan-only run via API
          tfc-manage-log-level: 'true'     # Injects TF_LOG_CORE=TRACE and restores afterwards
          upload-html-report: 'true'       # Saves interactive HTML report as artifact

> 💡 **Viewing Reports**:
> - The summary table is displayed in the GitHub Actions **Summary** tab.
> - The full interactive Gantt chart is available under **Artifacts** (`tf-analysis-report.html`).
```

#### 🔒 `TFC_TOKEN` Recommended Permissions

When running scheduled analysis (triggering runs and temporarily managing log level), configure the following minimum permissions for your Team Token:
- **Runs**: `Plan` or `Write` (required to queue and poll `plan-only` runs; `Apply` permission is **not** required).
- **Variables**: `Read & Write` (required to temporarily set and restore `TF_LOG_CORE`).
- **State Versions**: `No access` (state files are not read).

---

### Action Version Pinning & Checksum Integrity

To ensure maximum security and reproducibility, this Action strictly downloads official release binaries and validates their SHA-256 checksums from `checksums.txt`.

- **Semantic Version Pinning (Recommended for most teams)**:
  ```yaml
  uses: kheiakiyama/tf-graph-analysis@v0.1.7
  ```
- **Commit SHA Pinning (Recommended for strict security policies / OpenSSF Scorecard)**:
  ```yaml
  uses: kheiakiyama/tf-graph-analysis@<commit-sha> # v0.1.7
  ```
  When pinned by commit SHA, the Action automatically resolves the matching release tag from GitHub, downloads the official binary, and verifies its SHA-256 checksum. If pinning a non-release commit, specify `with: version: v0.1.7`.

---

## Example Output

### Terminal Summary
```text
================================================================================
                    Terraform Graph Critical Path Analysis                     
================================================================================
Total Estimated Execution Time: 2m22.729s
Total Nodes: 4114 | Critical Path Nodes: 19
--------------------------------------------------------------------------------

[ Critical Path (Bottleneck Chain) ]
Step | Resource Name                                                | Duration     | Share    | Offset      
---------------------------------------------------------------------------------------------------------
1    | module.resource_group                                        | 597ms        |    0.4% | 0s          
2    | module.resource_group.var.name                               | 3.692s       |    2.6% | 597ms       
3    | module.resource_group.azurerm_resource_group.main            | 5.865s       |    4.1% | 4.289s      
4    | module.network.azurerm_virtual_network.main                  | 4.046s       |    2.8% | 15.487s     
5    | module.network.azurerm_subnet.k8s                            | 16.763s      |   11.7% | 19.533s     
6    | module.aks.azurerm_kubernetes_cluster.main                   | 7.16s        |    5.0% | 44.724s     
7    | module.aks.module.aks_prometheus_monitor.azurerm_monitor_... | 31.193s      |   21.9% | 1m1.69s     
8    | helm_release.keda                                            | 41.288s      |   28.9% | 1m40.1s     
---------------------------------------------------------------------------------------------------------

[ Top 10 Slowest Resources ]
Rank | Resource Name                                                | Duration     | On CritPath | Slack     
---------------------------------------------------------------------------------------------------------
1    | helm_release.cert_manager                                    | 1m13.196s    | No         | 12.324s   
2    | helm_release.keda                                            | 41.288s      | YES (★)    | 0s        
3    | helm_release.ingress_nginx                                   | 34.778s      | No         | 53.34s    
```

### Real-World Insight
- **The slowest resource is not always the bottleneck**:
  `helm_release.cert_manager` took **1m 13s** (the slowest single item), but because it ran in parallel, it had **12.3 seconds of slack**. Optimizing it would not save any overall time.
- **The true sequential bottleneck**:
  `helm_release.keda` (**41.3s**) sat at the end of the sequential AKS cluster chain with **0s slack (YES ★)**. Splitting application/Helm resources into a separate state file immediately shaves **over 1 minute** off the infrastructure plan time.

---

## CLI Options

| Flag | Default | Description |
| --- | --- | --- |
| `-dot` | `""` | **(Required)** Path to Terraform graph DOT file |
| `-log` | `""` | Path to Terraform log file with `TRACE` or `DEBUG` logs |
| `-output-html` | `""` | Path to export standalone interactive HTML report |
| `-output-markdown` | `""` | Path to export GitHub-flavored Markdown report (for PR comments) |
| `-title` | `""` | Custom title/label displayed on reports (e.g. environment or workspace name) |
| `-output-trace` | `""` | Path to export Chrome Trace Event Format JSON |
| `-output-dot` | `""` | Path to export highlighted Graphviz DOT file |
| `-default-duration` | `100ms` | Fallback duration for resources without log timing |
| `-version` | `false` | Print version and exit |

---

## License

[MIT License](LICENSE) (c) 2026 kheiakiyama