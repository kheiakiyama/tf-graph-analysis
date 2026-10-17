package exporter

import (
	"encoding/json"
	"io"
	"os"
	"sort"
	"time"

	"github.com/kheiakiyama/tf-graph-analysis/pkg/model"
)

// TraceEvent represents a single event in Chrome Trace Event Format.
type TraceEvent struct {
	Name      string                 `json:"name"`
	Category  string                 `json:"cat"`
	Phase     string                 `json:"ph"` // "X" for Complete Event
	Timestamp int64                  `json:"ts"` // Microseconds
	Duration  int64                  `json:"dur"` // Microseconds
	PID       int                    `json:"pid"`
	TID       int                    `json:"tid"`
	CName     string                 `json:"cname,omitempty"` // Color: e.g. "bad", "good"
	Args      map[string]interface{} `json:"args,omitempty"`
}

// TraceData represents the top-level Chrome trace object.
type TraceData struct {
	TraceEvents []TraceEvent `json:"traceEvents"`
}

// ExportTraceJSON writes the Chrome Trace Event Format JSON to the writer.
func ExportTraceJSON(w io.Writer, res *model.AnalysisResult) error {
	var events []TraceEvent

	// Check if any node has actual StartTime from log
	var minLogTime time.Time
	hasRealTimestamps := false
	for _, n := range res.AllNodes {
		if !n.StartTime.IsZero() {
			if minLogTime.IsZero() || n.StartTime.Before(minLogTime) {
				minLogTime = n.StartTime
				hasRealTimestamps = true
			}
		}
	}

	// Simple thread assigner for simulated timeline
	// Tracks when each thread becomes free (in microseconds)
	var threadFreeUntil []int64

	// Sort nodes by start time for trace presentation
	sortedNodes := make([]*model.Node, len(res.AllNodes))
	copy(sortedNodes, res.AllNodes)
	sort.Slice(sortedNodes, func(i, j int) bool {
		if hasRealTimestamps && !sortedNodes[i].StartTime.IsZero() && !sortedNodes[j].StartTime.IsZero() {
			return sortedNodes[i].StartTime.Before(sortedNodes[j].StartTime)
		}
		return sortedNodes[i].EarliestStart < sortedNodes[j].EarliestStart
	})

	for _, n := range sortedNodes {
		var tsMicro int64
		var durMicro int64 = n.Duration.Microseconds()
		if durMicro == 0 {
			durMicro = 1 // at least 1 microsecond so it renders nicely
		}

		if hasRealTimestamps && !n.StartTime.IsZero() {
			tsMicro = n.StartTime.Sub(minLogTime).Microseconds()
		} else {
			tsMicro = n.EarliestStart.Microseconds()
		}

		// Allocate a virtual thread so overlapping resources don't collide
		tid := 1
		assigned := false
		for tIdx, freeUntil := range threadFreeUntil {
			if tsMicro >= freeUntil {
				threadFreeUntil[tIdx] = tsMicro + durMicro
				tid = tIdx + 1
				assigned = true
				break
			}
		}
		if !assigned {
			threadFreeUntil = append(threadFreeUntil, tsMicro+durMicro)
			tid = len(threadFreeUntil)
		}

		// Critical path nodes get highlighted in red ("bad" in chrome tracing cname)
		category := "resource"
		cname := ""
		if n.IsCritical {
			category = "critical"
			cname = "bad" // renders red in chrome://tracing
		}

		events = append(events, TraceEvent{
			Name:      n.Name,
			Category:  category,
			Phase:     "X",
			Timestamp: tsMicro,
			Duration:  durMicro,
			PID:       1,
			TID:       tid,
			CName:     cname,
			Args: map[string]interface{}{
				"duration_human": n.Duration.String(),
				"is_critical":    n.IsCritical,
				"slack":          n.Slack.String(),
				"type":           n.Type,
				"is_resource":    n.IsResource,
			},
		})
	}

	data := TraceData{TraceEvents: events}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

// ExportTraceJSONFile writes the trace events to a JSON file.
func ExportTraceJSONFile(path string, res *model.AnalysisResult) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return ExportTraceJSON(f, res)
}
