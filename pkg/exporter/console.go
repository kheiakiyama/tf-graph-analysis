package exporter

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/kheiakiyama/tf-graph-analysis/pkg/model"
)

// PrintConsoleReport writes a formatted analysis report to the writer.
func PrintConsoleReport(w io.Writer, res *model.AnalysisResult) {
	fmt.Fprintln(w, "================================================================================")
	fmt.Fprintln(w, "                    Terraform Graph Critical Path Analysis                     ")
	fmt.Fprintln(w, "================================================================================")
	fmt.Fprintf(w, "Total Estimated Execution Time: %v\n", res.TotalDuration)
	fmt.Fprintf(w, "Total Nodes: %d | Critical Path Nodes: %d\n", len(res.AllNodes), len(res.CriticalPath))
	fmt.Fprintln(w, "--------------------------------------------------------------------------------")

	fmt.Fprintln(w, "\n[ Critical Path (Bottleneck Chain) ]")
	fmt.Fprintf(w, "%-4s | %-60s | %-12s | %-8s | %-12s\n", "Step", "Resource Name", "Duration", "Share", "Offset")
	fmt.Fprintln(w, strings.Repeat("-", 105))

	var critTotal time.Duration
	for _, n := range res.CriticalPath {
		critTotal += n.Duration
	}

	for i, n := range res.CriticalPath {
		pct := 0.0
		if res.TotalDuration > 0 {
			pct = (float64(n.Duration) / float64(res.TotalDuration)) * 100
		}
		name := truncate(n.Name, 60)
		fmt.Fprintf(w, "%-4d | %-60s | %-12v | %6.1f%% | %-12v\n",
			i+1, name, n.Duration, pct, n.EarliestStart)
	}

	fmt.Fprintln(w, strings.Repeat("-", 105))

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

	fmt.Fprintf(w, "\n[ Top %d Slowest Resources ]\n", topN)
	fmt.Fprintf(w, "%-4s | %-60s | %-12s | %-10s | %-10s\n", "Rank", "Resource Name", "Duration", "On CritPath", "Slack")
	fmt.Fprintln(w, strings.Repeat("-", 105))

	for i := 0; i < topN; i++ {
		n := sortedByDuration[i]
		critMark := "No"
		if n.IsCritical {
			critMark = "YES (★)"
		}
		name := truncate(n.Name, 60)
		fmt.Fprintf(w, "%-4d | %-60s | %-12v | %-10s | %-10v\n",
			i+1, name, n.Duration, critMark, n.Slack)
	}
	fmt.Fprintln(w, "================================================================================")
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
