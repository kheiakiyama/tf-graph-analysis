package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/kheiakiyama/tf-graph-analysis/pkg/analyzer"
	"github.com/kheiakiyama/tf-graph-analysis/pkg/dot"
	"github.com/kheiakiyama/tf-graph-analysis/pkg/exporter"
	"github.com/kheiakiyama/tf-graph-analysis/pkg/logparser"
)

var (
	version = "0.1.0"
)

func main() {
	dotPath := flag.String("dot", "", "Path to Terraform graph DOT file (from `terraform graph -type=plan`) [REQUIRED]")
	logPath := flag.String("log", "", "Path to Terraform log file (from `TF_LOG=TRACE terraform plan/apply`) [OPTIONAL]")
	outputDot := flag.String("output-dot", "", "Path to output highlighted DOT file [OPTIONAL]")
	outputTrace := flag.String("output-trace", "", "Path to output Chrome Trace Event Format JSON file [OPTIONAL]")
	outputHtml := flag.String("output-html", "", "Path to output standalone interactive HTML report [OPTIONAL]")
	outputMarkdown := flag.String("output-markdown", "", "Path to output GitHub-flavored Markdown report [OPTIONAL]")
	defaultDuration := flag.Duration("default-duration", 100*time.Millisecond, "Default fallback duration for nodes without log timing")
	showVersion := flag.Bool("version", false, "Print version and exit")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "tf-graph-analysis (v%s) - Terraform Critical Path Analyzer\n\n", version)
		fmt.Fprintf(os.Stderr, "Usage: tf-graph-analysis -dot <graph.dot> [-log <tf.log>] [options]\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if *showVersion {
		fmt.Printf("tf-graph-analysis v%s\n", version)
		os.Exit(0)
	}

	if *dotPath == "" {
		fmt.Fprintln(os.Stderr, "Error: -dot argument is required.")
		flag.Usage()
		os.Exit(1)
	}

	// 1. Parse DOT file
	dotParser := dot.NewParser(false)
	g, err := dotParser.ParseFile(*dotPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing DOT file: %v\n", err)
		os.Exit(1)
	}

	// 2. Parse Log file if provided
	cpm := analyzer.NewAnalyzer(analyzer.Options{
		DefaultDuration: *defaultDuration,
	})

	if *logPath != "" {
		logParser := logparser.NewLogParser()
		timings, err := logParser.ParseFile(*logPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing log file: %v\n", err)
			os.Exit(1)
		}
		cpm.AttachTimings(g, timings)
	} else {
		// No log provided: attach default duration to all nodes
		cpm.AttachTimings(g, nil)
	}

	// 3. Run CPM analysis
	result, err := cpm.Analyze(g)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running critical path analysis: %v\n", err)
		os.Exit(1)
	}

	// 4. Output console report
	exporter.PrintConsoleReport(os.Stdout, result)

	// 5. Output highlighted DOT if requested
	if *outputDot != "" {
		if err := exporter.ExportHighlightedDOTFile(*outputDot, g); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing highlighted DOT: %v\n", err)
		} else {
			fmt.Printf("Highlighted DOT written to: %s\n", *outputDot)
		}
	}

	// 6. Output Chrome Trace JSON if requested
	if *outputTrace != "" {
		if err := exporter.ExportTraceJSONFile(*outputTrace, result); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing trace JSON: %v\n", err)
		} else {
			fmt.Printf("Chrome Trace JSON written to: %s\n(Open in chrome://tracing or https://ui.perfetto.dev)\n", *outputTrace)
		}
	}

	// 7. Output Standalone HTML Report if requested
	if *outputHtml != "" {
		if err := exporter.ExportHTMLReportFile(*outputHtml, result); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing HTML report: %v\n", err)
		} else {
			fmt.Printf("Interactive HTML report written to: %s\n(Completely local: open in any browser without internet)\n", *outputHtml)
		}
	}

	// 8. Output GitHub-flavored Markdown report if requested
	if *outputMarkdown != "" {
		if err := exporter.ExportMarkdownReportFile(*outputMarkdown, result); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing Markdown report: %v\n", err)
		} else {
			fmt.Printf("Markdown report written to: %s\n", *outputMarkdown)
		}
	}
}
