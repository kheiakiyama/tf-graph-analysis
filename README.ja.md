# tf-graph-analysis

> 🌐 [English Documentation](README.md)

Terraform の依存関係グラフ（DAG）と実際の実行ログから、**クリティカルパス分析（CPM: Critical Path Method）** を行い、パフォーマンスのボトルネックとなっている直列実行チェーンを可視化する CLI ツールです。

---

## 背景・モチベーション

Terraform を利用していて直面する大きな課題の一つが、管理リソースの肥大化に伴う `terraform plan` / `apply` の長時間化です。

Terraform は依存関係のないリソースを並列に処理しますが、暗黙的・明示的な依存関係があるリソース間では直列（シーケンシャル）な処理を行います。
大規模なインフラ環境では、「単に単体で実行時間が長いリソース」と「全体の工期を決定づけている直列依存チェーン（クリティカルパス）」が混在しており、どこを切り離せば高速化できるかを判断するのが困難でした。

このツールは、実際の Terraform 実行ログの所要時間をグラフにマッピングし、**全体の実行時間を決定づけているクリティカルパス（直列ボトルネック）を特定・可視化** することで、的確なリソース分割や State 分割を支援します。

---

## 主な機能

- **クリティカルパス（最長経路）の自動算出**:
  - トポロジカルソートと動的計画法（Forward/Backward Pass）により、全体のボトルネックチェーンと各ノードの余裕時間（Slack）を瞬時に計算。
- **多様な出力フォーマット**:
  1. **ターミナルコンソール表示**: クリティカルパスと最も時間のかかっているリソースのランキング。
  2. **完全ローカル自己完結型 HTML レポート (`-output-html`)**:
     - **外部通信ゼロ（No CDN, No External Requests）** のため、機密情報を含む社内データでも安全にブラウザで閲覧可能。
     - 時系列ウォーターフォール（ガントチャート）表示、リソース検索、0秒ノード非表示トグル、詳細ツールチップ。
  3. **Chrome Trace Event JSON (`-output-trace`)**:
     - `chrome://tracing` や [ui.perfetto.dev](https://ui.perfetto.dev) でガントチャート確認可能。
  4. **ハイライト付き Graphviz DOT (`-output-dot`)**:
     - クリティカルパス上のノードと辺を赤太線で強調した DOT ファイル。

---

## クイックスタート

### インストール・入手方法

#### 配布バイナリを直接ダウンロード
[Releases](https://github.com/kheiakiyama/tf-graph-analysis/releases) ページからお使いの OS（Linux, macOS, Windows）向けの最新アーカイブをダウンロードして展開してください。

> [!NOTE]
> **macOS をご利用の場合**: ダウンロードしたバイナリの初回実行時に Gatekeeper によりブロックされる場合があります。その場合は以下のコマンドで検疫属性を解除してください：
> ```bash
> xattr -d com.apple.quarantine ./tf-graph-analysis
> ```

#### Go で直接インストール
```bash
go install github.com/kheiakiyama/tf-graph-analysis/cmd/tf-graph-analysis@latest
```

#### ソースコードからビルド
```bash
git clone https://github.com/kheiakiyama/tf-graph-analysis.git
cd tf-graph-analysis
go build -o tf-graph-analysis ./cmd/tf-graph-analysis
```

### データの準備

分析には **「グラフファイル (DOT)」** と **「実行ログ (LOG)」** の 2 つが必要です。

#### 1. 依存グラフの出力 (DOT形式)
```bash
terraform graph -type=plan > graph.dot
```

#### 2. 実行ログの出力 (LOG形式)

実行環境によって取得方法が異なります：

##### A. ローカル実行の場合
```bash
TF_LOG=TRACE terraform plan -no-color > tf_plan.log 2>&1
```

##### B. HCP Terraform / Terraform Cloud / Enterprise（リモート実行）の場合
> [!IMPORTANT]
> リモート実行の場合、手元の端末で `TF_LOG=TRACE` を設定しても、クラウド側のコンテナには引き継がれません。  
> Terraform Cloud の Workspace 設定画面（**Variables**）で、以下の環境変数を登録してから plan を実行してください：
> - **Key**: `TF_LOG_CORE`
> - **Value**: `TRACE`
> - **Category**: `Environment variable`
> 
> その後、実行ログ（ダウンロードしたログファイル）を保存します。

---

### 解析の実行

```bash
# 基本的なコンソールサマリー表示
./tf-graph-analysis -dot graph.dot -log tf_plan.log

# すべての可視化フォーマットを同時出力
./tf-graph-analysis \
  -dot graph.dot \
  -log tf_plan.log \
  -output-html report.html \
  -output-trace trace.json \
  -output-dot highlighted.dot

# 完全ローカルでHTMLレポートを閲覧
open report.html
```

---

## GitHub Action による CI / Cron 連携

`tf-graph-analysis` は GitHub Actions に組み込んで実行できます。インフラ環境の構成と Terraform の実行方式に応じて、**2つの利用パターン** があります。

| パターン | 実行環境 | 想定トリガー | 特徴・使い分け |
|:---|:---|:---|:---|
| **パターン A: ローカル実行** | GitHub Actions ランナー上 | **CI (`pull_request`), Cron (`schedule`), 手動** | GHA 側で `terraform plan` を実行し、ログをローカルで取得。TFC Web GUI に影響を与えないため、全 PR での自動分析や Cron に最適。 |
| **パターン B: リモート実行** | HCP Terraform (TFC) | **Cron (`schedule`) のみ, 手動** | TFC リモート実行環境で実行された Plan ログを API 経由で取得して分析。**日常の Web GUI 表示を重くしないため、Cron による定期実行を強く推奨。** |

---

### パターン A: GitHub Actions ランナー上で直接 Terraform を実行している場合

GHA ランナー上で `terraform plan` を実行する場合、TRACE ログはランナー内部のファイルにのみ出力されるため、外部の GUI に一切影響を与えません。Pull Request ごとの CI や定期 Cron で安全かつ高速に実行できます。

```yaml
name: Terraform Plan & Critical Path Analysis

on:
  pull_request:
    branches: [ main ]
  schedule:
    # 毎週月曜 深夜 2:00 (JST)
    - cron: '0 17 * * 0'
  workflow_dispatch:

jobs:
  plan:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      pull-requests: write # PR コメント投稿に必要 (PR イベント時のみ)
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

> 💡 **レポートの出力先**:
> - **PR コメント**: Pull Request 起因の実行時のみ、サマリーが自動投稿されます。
> - **GitHub Actions Summary**: 実行画面の Summary タブに Markdown 表が常に表示されます。
> - **HTML レポート**: インタラクティブな HTML レポートはワークフローの Artifact（`terraform-critical-path-report`）として保存されます。
```

---

### パターン B: HCP Terraform (Terraform Cloud) でリモート実行している場合

HCP Terraform の Remote Execution モードを利用している環境では、Action が TFC API を介してログを取得し、依存関係グラフをリポジトリから生成して解析します。

#### ⚠️ TFC 利用時の重要な注意点（なぜ Cron のみを想定しているか）

1. **`TF_LOG_CORE: TRACE` の必要性**:
   - `terraform apply` と異なり、`terraform plan` はリソースの作成・変更を行わずリフレッシュ（状態確認）のみを行うため、標準のログにはリソースごとの所要時間が出力されません。
   - 正確なボトルネック分析を行うには、Terraform 内部のグラフ探索ログを出力する `TF_LOG_CORE=TRACE` の設定が必須です。
2. **TFC Web GUI への影響（日常の開発への支障）**:
   - TFC は実行ログをそのまま Web GUI のブラウザ画面にストリーミングします。
   - 数百〜1,000+ リソース規模の環境で `TF_LOG_CORE=TRACE` を常時有効化すると、数万行以上のログがブラウザに流れ込み、**TFC Web GUI がフリーズして差分の確認や Apply 判断に著しい時間がかかってしまいます**。
   - そのため、**PR オープンごとの自動実行は非推奨とし、深夜や週次の Cron（定期実行）での定点観測を強く推奨** しています。

#### 定期実行（Cron）ワークフローの設定例

本 Action は、定期実行時に以下の自動化を安全に行います：
- `tfc-manage-log-level: 'true'`: 実行開始時に TFC ワークスペース変数の状態を退避し、一時的に `TF_LOG_CORE=TRACE` をセット。**終了時にはエラー時も含めて必ず元の状態に復元（元々なければ削除）** します。
- `tfc-trigger-run: 'true'`: TFC API を叩いて、コミット起因の Webhook を待たずに安全な `plan-only` の Run を自動発行します。

```yaml
name: Scheduled Terraform Critical Path Analysis (TFC)

on:
  schedule:
    # 毎週月曜 深夜 2:00 (JST)
    - cron: '0 17 * * 0'
  workflow_dispatch: # 手動でのオンデマンド実行も可能

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
          tfc-trigger-run: 'true'          # API 経由で Speculative Plan (plan-only) を発行
          tfc-manage-log-level: 'true'     # 実行中のみ TF_LOG_CORE=TRACE を注入し事後復元
          upload-html-report: 'true'       # HTML レポートを Artifact に保存

> 💡 **分析結果の確認**:
> - 定期実行時、サマリー表は GitHub Actions 実行結果画面の **Summary** に表示されます。
> - 詳細なガントチャート・クリティカルパス可視化は、保存された **Artifacts** から `tf-analysis-report.html` をダウンロードしてブラウザで確認できます。
```

#### 🔒 `TFC_TOKEN` の推奨権限設定

定期実行（Run トリガーおよびログレベルの一時管理）を行う場合、Team Token または専用トークンに必要な最小限の権限を設定してください：
- **Runs**: `Plan` または `Write`（`plan-only` Run の作成・待機に必要。※ `Apply` 権限は **不要** です）
- **Variables**: `Read & Write`（`TF_LOG_CORE` の一時設定および復元に必要）
- **State Versions**: `No access`（State ファイル自体の読み取りは不要です）

---

### Action のバージョン指定とチェックサム検証

サプライチェーン攻撃の防止と実行の再現性を担保するため、本 Action は公式リリースバイナリのみをダウンロードし、`checksums.txt` の **SHA-256 チェックサムを自動検証** します。

- **セマンティックバージョン固定（一般的なチームに推奨）**:
  ```yaml
  uses: kheiakiyama/tf-graph-analysis@v0.1.7
  ```
- **コミット SHA ピン留め（厳格なセキュリティポリシー / OpenSSF Scorecard 準拠）**:
  ```yaml
  uses: kheiakiyama/tf-graph-analysis@<commit-sha> # v0.1.7
  ```
  コミット SHA でピン留めされた場合、Action が自動的に該当するリリースバージョンを GitHub API から逆引き解決し、バイナリのダウンロードとチェックサム検証を行います（未リリースコミットをピン留めする場合は `with: version: v0.1.7` を明示してください）。

---

## 実行結果の例

### ターミナル出力例
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

#### この分析から見えてくること
- 最も単体で遅い `helm_release.cert_manager`（1分13秒）は、並行実行されており **Slack（余裕時間）が 12.3秒** あるため、全体の完了時間を遅らせている犯人ではありません。
- 真のボトルネックは、直列に依存連鎖している末尾の **`helm_release.keda`（41.3秒、Slack 0s）** です。
- Helm や Prometheus モニターのリソースを別 State に切り離すことで、Plan 時間を大幅に短縮できることが客観的に証明されます。

---

## CLI オプション一覧

| フラグ | デフォルト | 説明 |
| --- | --- | --- |
| `-dot` | `""` | **(必須)** Terraform graph DOT ファイルのパス |
| `-log` | `""` | `TRACE` または `DEBUG` ログファイルのパス |
| `-output-html` | `""` | 完全オフライン動作の HTML レポート出力先パス |
| `-output-markdown` | `""` | GitHub Flavored Markdown レポート出力先パス (PR コメント用) |
| `-title` | `""` | レポートのタイトル/環境識別子（例: 環境名やワークスペース名） |
| `-output-trace` | `""` | Chrome Trace Event Format JSON 出力先パス |
| `-output-dot` | `""` | クリティカルパス強調 DOT ファイル出力先パス |
| `-default-duration` | `100ms` | ログに所要時間がないリソースのデフォルト推定時間 |
| `-version` | `false` | バージョンを表示して終了 |

---

## ライセンス

[MIT License](LICENSE) (c) 2026 kheiakiyama
