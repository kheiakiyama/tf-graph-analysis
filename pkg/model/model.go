package model

import (
	"time"
)

// Node represents a vertex in the Terraform execution graph.
type Node struct {
	ID             string        // Raw node ID from DOT (e.g., `[root] terraform_data.leaf (expand)`)
	Name           string        // Normalized resource address (e.g., `terraform_data.leaf`)
	Type           string        // Resource type or node category (e.g., `terraform_data`, `provider`, `meta`)
	IsResource     bool          // True if this is an actual terraform resource (not provider/meta)
	Duration       time.Duration // Execution duration (from log or estimated)
	StartTime      time.Time     // Execution start time (if found in log)
	EndTime        time.Time     // Execution end time (if found in log)
	HasTiming      bool          // True if actual timing was parsed from log

	// CPM (Critical Path Method) calculated values
	EarliestStart  time.Duration
	EarliestFinish time.Duration
	LatestStart    time.Duration
	LatestFinish   time.Duration
	Slack          time.Duration // LatestStart - EarliestStart (0 for critical nodes)
	IsCritical     bool          // True if part of the critical path
}

// Edge represents a directed dependency between two nodes.
// In the execution flow: From must finish before To can start.
type Edge struct {
	From       string // Predecessor node ID (must complete first)
	To         string // Successor node ID (depends on From)
	IsCritical bool   // True if this edge is on the critical path
}

// Graph represents the dependency graph of Terraform resources.
type Graph struct {
	Nodes            map[string]*Node
	Edges            []*Edge
	// Successors: node ID -> list of node IDs that depend on this node
	Successors       map[string][]string
	// Predecessors: node ID -> list of node IDs that this node depends on
	Predecessors     map[string][]string
}

// NewGraph creates a new empty Graph.
func NewGraph() *Graph {
	return &Graph{
		Nodes:        make(map[string]*Node),
		Edges:        make([]*Edge, 0),
		Successors:   make(map[string][]string),
		Predecessors: make(map[string][]string),
	}
}

// AddNode adds a node to the graph.
func (g *Graph) AddNode(node *Node) {
	g.Nodes[node.ID] = node
	if _, ok := g.Successors[node.ID]; !ok {
		g.Successors[node.ID] = make([]string, 0)
	}
	if _, ok := g.Predecessors[node.ID]; !ok {
		g.Predecessors[node.ID] = make([]string, 0)
	}
}

// AddEdge adds a directed execution edge from `from` (predecessor) to `to` (successor).
func (g *Graph) AddEdge(from, to string) {
	edge := &Edge{
		From: from,
		To:   to,
	}
	g.Edges = append(g.Edges, edge)
	g.Successors[from] = append(g.Successors[from], to)
	g.Predecessors[to] = append(g.Predecessors[to], from)
}

// AnalysisResult holds the summary of the critical path analysis.
type AnalysisResult struct {
	TotalDuration   time.Duration
	CriticalPath    []*Node
	AllNodes        []*Node
	CriticalEdges   []*Edge
}
