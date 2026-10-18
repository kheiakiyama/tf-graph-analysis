package exporter

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/kheiakiyama/tf-graph-analysis/pkg/model"
)

// PrintMarkdownReport writes a formatted GitHub-flavored Markdown report.
// Ideal for PR comments and GITHUB_STEP_SUMMARY.
func PrintMarkdownReport(w io.Writer, res *model.AnalysisResult, titleOpt ...string) {
	headerTitle := "### 📊 Terraform Graph Critical Path Analysis"
	if len(titleOpt) > 0 && strings.TrimSpace(titleOpt[0]) != "" {
		headerTitle = fmt.Sprintf("### 📊 Terraform Graph Critical Path Analysis (`%s`)", strings.TrimSpace(titleOpt[0]))
	}
	fmt.Fprintln(w, headerTitle)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "- **Estimated Total Execution Time:** `%v`\n", res.TotalDuration)
	fmt.Fprintf(w, "- **Total Graph Nodes:** `%d` (Critical Path Nodes: `%d`)\n", len(res.AllNodes), len(res.CriticalPath))
	fmt.Fprintln(w)

	// Critical Path Table
	fmt.Fprintln(w, "#### ⛓️ Critical Path (Bottleneck Chain)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| Step | Resource / Node | Duration | Share | Offset |")
	fmt.Fprintln(w, "|:---:|:---|:---:|:---:|:---:|")

	for i, n := range res.CriticalPath {
		pct := 0.0
		if res.TotalDuration > 0 {
			pct = (float64(n.Duration) / float64(res.TotalDuration)) * 100
		}
		// Escape backticks in resource names
		safeName := strings.ReplaceAll(n.Name, "`", "\\`")
		fmt.Fprintf(w, "| %d | `%s` | `%v` | %.1f%% | `%v` |\n",
			i+1, safeName, n.Duration, pct, n.EarliestStart)
	}
	fmt.Fprintln(w)

	// Top Slowest Resources
	sortedByDuration := make([]*model.Node, len(res.AllNodes))
	copy(sortedByDuration, res.AllNodes)
	sort.Slice(sortedByDuration, func(i, j int) bool {
		return sortedByDuration[i].Duration > sortedByDuration[j].Duration
	})

	topN := 10
	if len(sortedByDuration) < topN {
		topN = len(sortedByDuration)
	}

	fmt.Fprintln(w, "#### 🐢 Top 10 Slowest Resources")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| Rank | Resource / Node | Duration | On Critical Path | Slack (Float) |")
	fmt.Fprintln(w, "|:---:|:---|:---:|:---:|:---:|")

	for i := 0; i < topN; i++ {
		n := sortedByDuration[i]
		critMark := "No"
		if n.IsCritical {
			critMark = "**YES (★)**"
		}
		safeName := strings.ReplaceAll(n.Name, "`", "\\`")
		fmt.Fprintf(w, "| %d | `%s` | `%v` | %s | `%v` |\n",
			i+1, safeName, n.Duration, critMark, n.Slack)
	}
	fmt.Fprintln(w)

	// Note on Critical Path vs Slowest
	fmt.Fprintln(w, "> **Note:** Resources marked with **YES (★)** directly dictate the minimum execution time. Speeding up non-critical resources with positive slack will **not** reduce total execution time unless their duration exceeds their slack.")
}

// ExportMarkdownReportFile writes the Markdown report to a file path.
func ExportMarkdownReportFile(filePath string, res *model.AnalysisResult, titleOpt ...string) error {
	var sb strings.Builder
	PrintMarkdownReport(&sb, res, titleOpt...)
	return os.WriteFile(filePath, []byte(sb.String()), 0644)
}
