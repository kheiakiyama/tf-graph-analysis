package dot_test

import (
	"path/filepath"
	"testing"

	"github.com/kheiakiyama/tf-graph-analysis/pkg/dot"
)

func TestParseSampleGraph(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "testdata", "fixtures", "sample_graph.dot")
	parser := dot.NewParser(false)
	g, err := parser.ParseFile(fixturePath)
	if err != nil {
		t.Fatalf("failed to parse sample graph: %v", err)
	}

	if len(g.Nodes) == 0 {
		t.Fatal("expected nodes, got 0")
	}

	// Verify leaf node exists
	leafID := `[root] terraform_data.leaf (expand)`
	node, exists := g.Nodes[leafID]
	if !exists {
		t.Fatalf("node %s not found", leafID)
	}
	if !node.IsResource {
		t.Errorf("expected leaf to be resource, got false")
	}
	if node.Name != "terraform_data.leaf" {
		t.Errorf("expected node name 'terraform_data.leaf', got '%s'", node.Name)
	}

	// In sample_graph:
	// "[root] terraform_data.leaf (expand)" -> "[root] terraform_data.slow_step_2 (expand)"
	// Execution edge should be slow_step_2 -> leaf
	slowStep2ID := `[root] terraform_data.slow_step_2 (expand)`
	predecessors := g.Predecessors[leafID]
	foundSlowStep2 := false
	for _, p := range predecessors {
		if p == slowStep2ID {
			foundSlowStep2 = true
			break
		}
	}
	if !foundSlowStep2 {
		t.Errorf("expected %s to be predecessor of %s, but wasn't found in %v", slowStep2ID, leafID, predecessors)
	}
}
