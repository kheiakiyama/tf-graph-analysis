package logparser_test

import (
	"path/filepath"
	"testing"

	"github.com/kheiakiyama/tf-graph-analysis/pkg/logparser"
)

func TestParseSampleLog(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "testdata", "fixtures", "sample_plan.log")
	parser := logparser.NewLogParser()
	timings, err := parser.ParseFile(fixturePath)
	if err != nil {
		t.Fatalf("failed to parse sample log: %v", err)
	}

	if len(timings) == 0 {
		t.Fatal("expected timings, got 0")
	}

	// Verify terraform_data.root_a timing is parsed
	rootA, exists := timings["terraform_data.root_a"]
	if !exists {
		t.Fatalf("timing for terraform_data.root_a not found in %v", timings)
	}

	if rootA.StartTime.IsZero() {
		t.Errorf("expected non-zero start time for terraform_data.root_a")
	}
	if rootA.EndTime.IsZero() {
		t.Errorf("expected non-zero end time for terraform_data.root_a")
	}
	t.Logf("terraform_data.root_a duration: %v, visits: %d", rootA.Duration, rootA.VisitCount)
}
