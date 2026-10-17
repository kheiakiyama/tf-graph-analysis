package exporter

import (
	"fmt"
	"io"
	"os"

	"github.com/kheiakiyama/tf-graph-analysis/pkg/model"
)

// ExportHighlightedDOT writes a Graphviz DOT file with critical path elements highlighted in red.
func ExportHighlightedDOT(w io.Writer, g *model.Graph) error {
	fmt.Fprintln(w, "digraph G {")
	fmt.Fprintln(w, "  rankdir = \"TB\";")
	fmt.Fprintln(w, "  compound = \"true\";")
	fmt.Fprintln(w, "  node [fontname=\"sans-serif\", fontsize=10, shape=\"box\"];")
	fmt.Fprintln(w, "  edge [fontname=\"sans-serif\", fontsize=9];")

	// Write nodes
	for _, node := range g.Nodes {
		label := fmt.Sprintf("%s\\n(%v)", node.Name, node.Duration)
		if node.IsCritical {
			fmt.Fprintf(w, "  %q [label=%q, color=\"#d9381e\", fillcolor=\"#ffebee\", style=\"filled,bold\", penwidth=2.0];\n",
				node.ID, label)
		} else {
			color := "#888888"
			if node.IsResource {
				color = "#1976d2"
			}
			fmt.Fprintf(w, "  %q [label=%q, color=%q];\n",
				node.ID, label, color)
		}
	}

	// Write edges (execution flow: From -> To)
	for _, edge := range g.Edges {
		if edge.IsCritical {
			fmt.Fprintf(w, "  %q -> %q [color=\"#d9381e\", penwidth=2.5, style=\"bold\"];\n",
				edge.From, edge.To)
		} else {
			fmt.Fprintf(w, "  %q -> %q [color=\"#cccccc\"];\n",
				edge.From, edge.To)
		}
	}

	fmt.Fprintln(w, "}")
	return nil
}

// ExportHighlightedDOTFile writes highlighted DOT to a file path.
func ExportHighlightedDOTFile(path string, g *model.Graph) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return ExportHighlightedDOT(f, g)
}
