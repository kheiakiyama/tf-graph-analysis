package analyzer_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/kheiakiyama/tf-graph-analysis/pkg/analyzer"
	"github.com/kheiakiyama/tf-graph-analysis/pkg/dot"
	"github.com/kheiakiyama/tf-graph-analysis/pkg/logparser"
	"github.com/kheiakiyama/tf-graph-analysis/pkg/model"
)

func TestCPMCalculation(t *testing.T) {
	// Build a known DAG:
	// A (10s) -> B (20s) -> D (5s) => Path 1 duration: 10 + 20 + 5 = 35s (Critical)
	// A (10s) -> C (5s)  -> D (5s) => Path 2 duration: 10 + 5 + 5  = 20s (Slack for C = 15s)
	g := model.NewGraph()

	nodeA := &model.Node{ID: "A", Name: "A", Duration: 10 * time.Second}
	nodeB := &model.Node{ID: "B", Name: "B", Duration: 20 * time.Second}
	nodeC := &model.Node{ID: "C", Name: "C", Duration: 5 * time.Second}
	nodeD := &model.Node{ID: "D", Name: "D", Duration: 5 * time.Second}

	g.AddNode(nodeA)
	g.AddNode(nodeB)
	g.AddNode(nodeC)
	g.AddNode(nodeD)

	g.AddEdge("A", "B")
	g.AddEdge("B", "D")
	g.AddEdge("A", "C")
	g.AddEdge("C", "D")

	cpm := analyzer.NewAnalyzer(analyzer.Options{})
	res, err := cpm.Analyze(g)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	expectedTotal := 35 * time.Second
	if res.TotalDuration != expectedTotal {
		t.Errorf("expected total duration %v, got %v", expectedTotal, res.TotalDuration)
	}

	// Verify Slack for C
	if nodeC.Slack != 15*time.Second {
		t.Errorf("expected Slack for C to be 15s, got %v", nodeC.Slack)
	}
	if nodeC.IsCritical {
		t.Errorf("node C should not be critical")
	}

	// Verify Critical Path is [A, B, D]
	if len(res.CriticalPath) != 3 {
		t.Fatalf("expected critical path length 3, got %d", len(res.CriticalPath))
	}
	expectedChain := []string{"A", "B", "D"}
	for i, n := range res.CriticalPath {
		if n.ID != expectedChain[i] {
			t.Errorf("expected chain[%d] = %s, got %s", i, expectedChain[i], n.ID)
		}
	}
}

func TestEndToEndWithFixtures(t *testing.T) {
	dotPath := filepath.Join("..", "..", "testdata", "fixtures", "sample_graph.dot")
	logPath := filepath.Join("..", "..", "testdata", "fixtures", "sample_plan.log")

	dotParser := dot.NewParser(false)
	g, err := dotParser.ParseFile(dotPath)
	if err != nil {
		t.Fatalf("failed to parse dot: %v", err)
	}

	logParser := logparser.NewLogParser()
	timings, err := logParser.ParseFile(logPath)
	if err != nil {
		t.Fatalf("failed to parse log: %v", err)
	}

	cpm := analyzer.NewAnalyzer(analyzer.Options{DefaultDuration: 1 * time.Millisecond})
	cpm.AttachTimings(g, timings)

	res, err := cpm.Analyze(g)
	if err != nil {
		t.Fatalf("failed to analyze: %v", err)
	}

	if len(res.CriticalPath) == 0 {
		t.Fatal("expected non-empty critical path")
	}
	t.Logf("Total Duration: %v", res.TotalDuration)
	t.Logf("Critical Path Length: %d", len(res.CriticalPath))
	for i, node := range res.CriticalPath {
		t.Logf("  [%d] %s (%v)", i+1, node.Name, node.Duration)
	}
}
