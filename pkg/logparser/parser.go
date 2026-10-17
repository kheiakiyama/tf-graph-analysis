package logparser

import (
	"bufio"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
)

// Common timestamp formats used in Terraform logs
const (
	tfTimeFormatMilli = "2006-01-02T15:04:05.000-0700"
	tfTimeFormatMicro = "2006-01-02T15:04:05.000000-0700"
	tfTimeFormatRFC   = time.RFC3339Nano
)

var (
	// Matches: 2026-10-18T06:55:49.787+0900 [TRACE] vertex "terraform_data.root_a": starting visit (*terraform.NodePlannableResourceInstance)
	traceStartRegex = regexp.MustCompile(`^(\S+)\s+\[TRACE\]\s+vertex\s+"([^"]+)":\s+starting visit\s+\(([^)]+)\)`)

	// Matches: 2026-10-18T06:55:49.788+0900 [TRACE] vertex "terraform_data.root_a": visit complete
	traceEndRegex = regexp.MustCompile(`^(\S+)\s+\[TRACE\]\s+vertex\s+"([^"]+)":\s+visit complete`)

	// Matches UI lines like: aws_instance.web: Creation complete after 15s [id=...]
	uiCompleteRegex = regexp.MustCompile(`^(\S+):\s+(?:Creation|Modifications|Destruction|Read)\s+complete after\s+(\d+[smh\.]+)`)
)

// ResourceTiming holds timing data for a specific resource.
type ResourceTiming struct {
	ResourceName string
	StartTime    time.Time
	EndTime      time.Time
	Duration     time.Duration
	VisitCount   int
}

// LogParser extracts resource execution timings from Terraform logs.
type LogParser struct{}

// NewLogParser creates a new LogParser.
func NewLogParser() *LogParser {
	return &LogParser{}
}

// ParseFile parses a Terraform log file from disk.
func (p *LogParser) ParseFile(path string) (map[string]*ResourceTiming, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return p.Parse(f)
}

// Parse reads a Terraform log and returns a map of resource names to timings.
func (p *LogParser) Parse(r io.Reader) (map[string]*ResourceTiming, error) {
	scanner := bufio.NewScanner(r)
	// Some log lines can be very large
	const maxCapacity = 10 * 1024 * 1024
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	timings := make(map[string]*ResourceTiming)

	// Keep track of pending start events for each vertex
	// Key: resourceName (cleaned)
	type pendingVisit struct {
		startTime time.Time
		nodeType  string
	}
	pending := make(map[string][]pendingVisit)

	for scanner.Scan() {
		line := scanner.Text()

		// 1. Check TRACE starting visit
		if m := traceStartRegex.FindStringSubmatch(line); m != nil {
			tsStr := m[1]
			rawVertex := m[2]
			nodeType := m[3]

			t, err := parseTimestamp(tsStr)
			if err != nil {
				continue
			}

			cleanName := cleanVertexName(rawVertex)
			pending[cleanName] = append(pending[cleanName], pendingVisit{
				startTime: t,
				nodeType:  nodeType,
			})
			continue
		}

		// 2. Check TRACE visit complete
		if m := traceEndRegex.FindStringSubmatch(line); m != nil {
			tsStr := m[1]
			rawVertex := m[2]

			t, err := parseTimestamp(tsStr)
			if err != nil {
				continue
			}

			cleanName := cleanVertexName(rawVertex)
			visits := pending[cleanName]
			if len(visits) > 0 {
				// Pop the last visit (FIFO or LIFO, typically matched)
				lastIdx := len(visits) - 1
				visit := visits[lastIdx]
				pending[cleanName] = visits[:lastIdx]

				dur := t.Sub(visit.startTime)
				if dur < 0 {
					dur = 0
				}

				timing, exists := timings[cleanName]
				if !exists {
					timing = &ResourceTiming{
						ResourceName: cleanName,
						StartTime:    visit.startTime,
						EndTime:      t,
						Duration:     dur,
						VisitCount:   1,
					}
					timings[cleanName] = timing
				} else {
					// If resource visited multiple times (e.g. validate, then plan/apply),
					// keep the one with longer duration or expand start/end span.
					timing.VisitCount++
					if dur > timing.Duration {
						timing.Duration = dur
						timing.StartTime = visit.startTime
						timing.EndTime = t
					}
				}
			}
			continue
		}

		// 3. Fallback: Check standard UI output (e.g. "aws_instance.web: Creation complete after 12s")
		if m := uiCompleteRegex.FindStringSubmatch(line); m != nil {
			cleanName := cleanVertexName(m[1])
			durStr := m[2]
			if dur, err := time.ParseDuration(durStr); err == nil {
				if existing, ok := timings[cleanName]; !ok || existing.Duration < dur {
					timings[cleanName] = &ResourceTiming{
						ResourceName: cleanName,
						Duration:     dur,
						VisitCount:   1,
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return timings, nil
}

func parseTimestamp(ts string) (time.Time, error) {
	if t, err := time.Parse(tfTimeFormatMilli, ts); err == nil {
		return t, nil
	}
	if t, err := time.Parse(tfTimeFormatMicro, ts); err == nil {
		return t, nil
	}
	if t, err := time.Parse(tfTimeFormatRFC, ts); err == nil {
		return t, nil
	}
	return time.Time{}, os.ErrInvalid
}

func cleanVertexName(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "[root] ")
	s = strings.TrimSuffix(s, " (expand)")
	s = strings.TrimSuffix(s, " (close)")
	return s
}
