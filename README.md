# tf-graph-analysis

## Motivation
terraform を利用していて痛みに感じることの一つが、管理リソースの肥大化に伴い実行時間が長期化することにある  
これは背景として、terraform では依存関係があるリソース間でシーケンシャルな処理を実行する仕組みになっている  
このツールは実際の terraform 実行時間をもとにパフォーマンスのボトルネックとなるグラフのクリティカルパスを可視化することで、的確なリソースの分割をサポートできるのではないかと考えた  

## How it works

1. **`terraform graph -type=plan` の DOT ファイルをパース**: Terraform Core が解釈したリソース間の DAG（依存グラフ）を構築します。
2. **`TF_LOG=TRACE` ログから所要時間を抽出**: 各リソースの `vertex "...": starting visit` から `visit complete` までの実測ミリ秒（またはUI完了ログ）をパースして各ノードの重み（Duration）として付与します。
3. **クリティカルパス分析（CPM: Critical Path Method）の実行**:
   - トポロジカルソート & 動的計画法（Earliest Start / Finish）により全体の最長経路を計算。
   - バックトラックによって全体のボトルネックとなっている直列リソース列（クリティカルパス）を特定。
   - 各リソースの Slack（余裕時間）を算出。
4. **多彩な出力形式**:
   - **コンソール表示**: クリティカルパスと最も時間のかかっているリソースのランキング。
   - **ハイライト付き DOT**: クリティカルパスを赤くハイライトした Graphviz DOT ファイル（SVG/PNG化可能）。
   - **Chrome Trace Event JSON**: [ui.perfetto.dev](https://ui.perfetto.dev) や `chrome://tracing` でドラッグ＆ドロップして並列実行と直列待ちをガントチャート確認可能。

## Quick Start

### インストール・ビルド
```bash
go build -o tf-graph-analysis ./cmd/tf-graph-analysis
```

### データの準備
対象の Terraform ディレクトリで以下を実行してデータを出力します：

```bash
# 1. 依存グラフの出力 (DOT形式)
terraform graph -type=plan > graph.dot

# 2. 実行ログの出力 (TRACEログ)
TF_LOG=TRACE terraform plan -no-color > tf_plan.log 2>&1
```

### 解析の実行
```bash
# 基本的なコンソールサマリー表示
./tf-graph-analysis -dot graph.dot -log tf_plan.log

# ハイライト付き DOT と Chrome Tracing JSON を同時出力
./tf-graph-analysis \
  -dot graph.dot \
  -log tf_plan.log \
  -output-dot highlighted.dot \
  -output-trace trace.json
```

---

## Plan by Gemini

「静的な依存グラフ（DAG）」に「各ノードの実行時間」を重み付けして合体させれば、**クリティカルパス分析（CPM: Critical Path Method）** を使って最長パスを特定できます。

ツールを成立させるためのデータソース、クリティカルパスの計算手法、および最小限の実装アプローチを整理します。

---

### 1. 必要なデータと取得元

クリティカルパスを割り出すには、**「ノード間の接続関係（有向辺）」** と **「各ノードの処理所要時間（重み）」** の2つが必要です。

| 必要な要素 | 取得元 | 取得方法・特徴 |
| --- | --- | --- |
| **グラフ構造（DAG）** | `terraform graph` または HCL / State | `terraform graph -type=plan` の DOT 出力を使うのが最も正確。Core が実際に解釈した依存順がそのまま手に入る。 |
| **各リソースの所要時間** | `TF_LOG=DEBUG` / `TRACE` のログ | リクエスト/レスポンスのタイムスタンプ、または `ApplyResource` / `ReadResource` の開始・終了時刻からミリ秒単位で抽出。 |

> **Stateファイル単体で完結できるか？**
> State（JSON）内にも `dependencies` 配列が記録されているため簡易的なトポロジーは組めます。ただし、実際の Plan 時に評価される暗黙の参照関係や Provider ノードの依存関係が一部間引かれているため、**`terraform graph` の DOT ファイルをベースにするのが最も精度が高い**です。

---

### 2. クリティカルパス特定のアルゴリズム

Terraform のグラフはサイクルを持たない有向非巡回グラフ（DAG）であるため、**動的計画法（DAGの最長経路問題）** で $O(V + E)$ の線形時間で一瞬で計算できます。

1. **トポロジカルソート:** ノードを依存順にソートする。
2. **最長パス計算（Earliest Finish Time）:**
* 親を持たない始点ノードの開始時刻を 0 とする。
* 各ノード $u$ について、その完了時刻 $E(u) = S(u) + \text{duration}(u)$
* 子ノード $v$ の開始可能時刻 $S(v) = \max_{u \in \text{parents}(v)} E(u)$


3. **バックトラック:**
* 最も遅く完了した終点ノードから、最大値を供給し続けた親ノードを逆順に辿ることで、**全体の実行時間を決定づけている「クリティカルパス（直列のボトルネックチェーン）」** が特定できます。



---

### 3. ツールのアーキテクチャ・実装イメージ

Go または Python で CLI ツールとして構築するのが最も軽量です。

```
[CI / ローカル実行]
  ├── terraform graph -type=plan > graph.dot
  └── TF_LOG=TRACE terraform plan 2>&1 > tf.log
            │
            ▼
[CLI ツール (analyzer)]
  1. graph.dot をパース (gonum/graph や networkx)
  2. tf.log から各リソースの Read/Refresh 所要時間を抽出
  3. 各ノードに duration を付与し、クリティカルパスを逆算
            │
            ▼
[出力]
  - コンソール出力: ボトルネックの上位チェーン
  - 可視化: 
      - DOT形式（クリティカルパスの辺とノードを赤くハイライト）
      - HTML/SVG (Mermaid.js や d3.js によるインタラクティブ可視化)
      - Chrome Trace Event 形式 (chrome://tracing や Perfetto でガントチャート表示)

```

#### Chrome Tracing 形式への変換（ガントチャート表示）

特におすすめなのが **Chrome Trace Event Format (JSON)** への出力です。
各リソースの「開始時刻」「終了時刻」「スレッド（並列ワーカー）」を JSON に吐き出すだけで、ブラウザ（`chrome://tracing` や `ui.perfetto.dev`）にドラッグ＆ドロップして、1000 リソースがどのように並列実行され、どこで直列待ちが発生したのかをタイムライン上で視覚的に確認できます。

---

### 4. このツールを作ると見えてくること

1000 リソースの Azure 環境でこの可視化を行うと、ほぼ確実に以下のパターンが炙り出されます。

* **アンカーとなっている「重い単一リソース」:**
* 例: `azurerm_kubernetes_cluster` や `azurerm_application_gateway` の Read が 1 つで 15 秒かかっており、その下流にある数十個のリソースがすべて足止めされている様子。


* **不要な暗黙の依存関係（直列化の犯人）:**
* 「リソースグループの ID」を参照しているせいで、本来は互いに無関係なリソース同士が数珠つなぎの直列パスになってしまっている箇所。
* （例: `azurerm_resource_group.rg.name` をベタ書き文字列にするか Local 変数に逃がすだけで、依存グラフが切り離されて並列実行できるようになるポイント）。


* **State 分割の境界線:**
* グラフの結合度が低く、独立してクラスター化しているサブグラフが視覚的に浮き彫りになるため、「どこで State を切り離すべきか」の客観的な判断材料になる。



自作ツールとしての難易度も高すぎず、既存の `terraform-visual` や `tfgraph` などのツールには「実行時間のプロファイリングとクリティカルパスのハイライト」まで統合したものはほぼ存在しないため、OSS や社内ツールとしても非常に価値のある試みになります。