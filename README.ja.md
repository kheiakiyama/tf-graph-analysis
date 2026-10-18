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

### インストール・ビルド

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

## ライセンス

[MIT License](LICENSE) (c) 2026 kheiakiyama
