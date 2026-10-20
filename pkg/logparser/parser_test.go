package logparser_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestParseEscapedQuotes(t *testing.T) {
	logContent := `2026-10-20T10:13:37.066Z [TRACE] vertex "aws_instance.server[\"web-1\"]": starting visit (*terraform.NodePlannableResourceInstance)
2026-10-20T10:13:40.066Z [TRACE] vertex "aws_instance.server[\"web-1\"]": visit complete
data.external.example["Cluster Beta"]: Refresh complete after 4s [id=test]
`
	parser := logparser.NewLogParser()
	timings, err := parser.Parse(strings.NewReader(logContent))
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	server1, ok := timings[`aws_instance.server["web-1"]`]
	if !ok {
		t.Fatalf("expected aws_instance.server[\"web-1\"], got keys: %v", timings)
	}
	if server1.Duration != 3*time.Second {
		t.Errorf("expected 3s duration, got %v", server1.Duration)
	}

	example, ok := timings[`data.external.example["Cluster Beta"]`]
	if !ok {
		t.Fatalf("expected data.external.example[\"Cluster Beta\"], got keys: %v", timings)
	}
	if example.Duration != 4*time.Second {
		t.Errorf("expected 4s duration, got %v", example.Duration)
	}
}
