package exporter_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kheiakiyama/tf-graph-analysis/pkg/exporter"
	"github.com/kheiakiyama/tf-graph-analysis/pkg/model"
)

func TestConsoleReport(t *testing.T) {
	nodeA := &model.Node{ID: "A", Name: "A", Duration: 10 * time.Second, IsCritical: true}
	res := &model.AnalysisResult{
		TotalDuration: 10 * time.Second,
		CriticalPath:  []*model.Node{nodeA},
		AllNodes:      []*model.Node{nodeA},
	}

	var buf bytes.Buffer
	exporter.PrintConsoleReport(&buf, res)
	output := buf.String()

	if !strings.Contains(output, "Critical Path (Bottleneck Chain)") {
		t.Errorf("expected header in output")
	}
	if !strings.Contains(output, "A") {
		t.Errorf("expected node A in output")
	}
}

func TestTraceJSON(t *testing.T) {
	nodeA := &model.Node{ID: "A", Name: "res_a", Duration: 5 * time.Second, IsCritical: true}
	res := &model.AnalysisResult{
		TotalDuration: 5 * time.Second,
		CriticalPath:  []*model.Node{nodeA},
		AllNodes:      []*model.Node{nodeA},
	}

	var buf bytes.Buffer
	err := exporter.ExportTraceJSON(&buf, res)
	if err != nil {
		t.Fatalf("ExportTraceJSON failed: %v", err)
	}

	var traceData exporter.TraceData
	if err := json.Unmarshal(buf.Bytes(), &traceData); err != nil {
		t.Fatalf("unmarshal json failed: %v", err)
	}

	if len(traceData.TraceEvents) != 1 {
		t.Fatalf("expected 1 event, got %d", len(traceData.TraceEvents))
	}
	if traceData.TraceEvents[0].Name != "res_a" {
		t.Errorf("expected name res_a, got %s", traceData.TraceEvents[0].Name)
	}
	if traceData.TraceEvents[0].CName != "bad" {
		t.Errorf("expected critical node to have cname 'bad', got '%s'", traceData.TraceEvents[0].CName)
	}
}

func TestHTMLReport(t *testing.T) {
	nodeA := &model.Node{ID: "A", Name: "res_a", Duration: 5 * time.Second, IsCritical: true}
	res := &model.AnalysisResult{
		TotalDuration: 5 * time.Second,
		CriticalPath:  []*model.Node{nodeA},
		AllNodes:      []*model.Node{nodeA},
	}

	var buf bytes.Buffer
	err := exporter.ExportHTMLReport(&buf, res)
	if err != nil {
		t.Fatalf("ExportHTMLReport failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Terraform Critical Path Report") {
		t.Errorf("expected HTML title in output")
	}
	if !strings.Contains(output, "res_a") {
		t.Errorf("expected res_a in output")
	}
}

func TestMarkdownReport(t *testing.T) {
	nodeA := &model.Node{ID: "A", Name: "res_a", Duration: 5 * time.Second, IsCritical: true}
	res := &model.AnalysisResult{
		TotalDuration: 5 * time.Second,
		CriticalPath:  []*model.Node{nodeA},
		AllNodes:      []*model.Node{nodeA},
	}

	var buf bytes.Buffer
	exporter.PrintMarkdownReport(&buf, res)
	output := buf.String()

	if !strings.Contains(output, "### 📊 Terraform Graph Critical Path Analysis") {
		t.Errorf("expected markdown header in output")
	}
	if !strings.Contains(output, "`res_a`") {
		t.Errorf("expected `res_a` in markdown output")
	}
}

