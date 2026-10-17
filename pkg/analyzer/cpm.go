package analyzer

import (
	"fmt"
	"sort"
	"time"

	"github.com/kheiakiyama/tf-graph-analysis/pkg/logparser"
	"github.com/kheiakiyama/tf-graph-analysis/pkg/model"
)

// Options holds configuration for the CPM analysis.
type Options struct {
	DefaultDuration time.Duration // Fallback duration for nodes with no log timing (default: 0)
}

// Analyzer performs Critical Path Method analysis on a Terraform graph.
type Analyzer struct {
	opts Options
}

// NewAnalyzer creates a new Analyzer.
func NewAnalyzer(opts Options) *Analyzer {
	return &Analyzer{
		opts: opts,
	}
}

// AttachTimings attaches parsed log timings to the corresponding graph nodes.
func (a *Analyzer) AttachTimings(g *model.Graph, timings map[string]*logparser.ResourceTiming) {
	for _, node := range g.Nodes {
		if timing, ok := timings[node.Name]; ok {
			node.Duration = timing.Duration
			node.StartTime = timing.StartTime
			node.EndTime = timing.EndTime
			node.HasTiming = true
		} else {
			node.Duration = a.opts.DefaultDuration
			node.HasTiming = false
		}
	}
}

// Analyze calculates the critical path and returns an AnalysisResult.
func (a *Analyzer) Analyze(g *model.Graph) (*model.AnalysisResult, error) {
	// 1. Topological Sort (Kahn's algorithm)
	sortedNodeIDs, err := a.topologicalSort(g)
	if err != nil {
		return nil, fmt.Errorf("cycle detected or invalid graph: %w", err)
	}

	// 2. Forward Pass: Calculate EarliestStart (ES) and EarliestFinish (EF)
	for _, id := range sortedNodeIDs {
		node := g.Nodes[id]
		var maxPredEF time.Duration = 0

		preds := g.Predecessors[id]
		for _, predID := range preds {
			pred := g.Nodes[predID]
			if pred.EarliestFinish > maxPredEF {
				maxPredEF = pred.EarliestFinish
			}
		}

		node.EarliestStart = maxPredEF
		node.EarliestFinish = node.EarliestStart + node.Duration
	}

	// 3. Find total duration and max finish time
	var totalDuration time.Duration = 0
	for _, node := range g.Nodes {
		if node.EarliestFinish > totalDuration {
			totalDuration = node.EarliestFinish
		}
	}

	// 4. Backward Pass: Calculate LatestFinish (LF) and LatestStart (LS)
	// Iterate in reverse topological order
	for i := len(sortedNodeIDs) - 1; i >= 0; i-- {
		id := sortedNodeIDs[i]
		node := g.Nodes[id]
		succs := g.Successors[id]

		if len(succs) == 0 {
			// Terminal node
			node.LatestFinish = totalDuration
		} else {
			var minSuccLS time.Duration = totalDuration
			for _, succID := range succs {
				succ := g.Nodes[succID]
				if succ.LatestStart < minSuccLS {
					minSuccLS = succ.LatestStart
				}
			}
			node.LatestFinish = minSuccLS
		}

		node.LatestStart = node.LatestFinish - node.Duration
		node.Slack = node.LatestStart - node.EarliestStart
		if node.Slack < 0 {
			node.Slack = 0
		}
		// A node is critical if Slack == 0 (with small float tolerance if needed)
		if node.Slack == 0 {
			node.IsCritical = true
		}
	}

	// 5. Backtrack to find the critical path chain
	// Pick the terminal node with max EarliestFinish that is critical
	criticalNodes := a.extractCriticalPathChain(g, totalDuration)

	// Mark critical edges
	critMap := make(map[string]bool)
	for _, n := range criticalNodes {
		critMap[n.ID] = true
	}

	var criticalEdges []*model.Edge
	for _, edge := range g.Edges {
		if critMap[edge.From] && critMap[edge.To] {
			fromNode := g.Nodes[edge.From]
			toNode := g.Nodes[edge.To]
			// Edge is on the critical path if From finishes exactly when To starts
			if fromNode.EarliestFinish == toNode.EarliestStart {
				edge.IsCritical = true
				criticalEdges = append(criticalEdges, edge)
			}
		}
	}

	// Collect all nodes sorted by EarliestStart
	allNodes := make([]*model.Node, 0, len(g.Nodes))
	for _, node := range g.Nodes {
		allNodes = append(allNodes, node)
	}
	sort.Slice(allNodes, func(i, j int) bool {
		if allNodes[i].EarliestStart != allNodes[j].EarliestStart {
			return allNodes[i].EarliestStart < allNodes[j].EarliestStart
		}
		return allNodes[i].Name < allNodes[j].Name
	})

	return &model.AnalysisResult{
		TotalDuration: totalDuration,
		CriticalPath:  criticalNodes,
		AllNodes:      allNodes,
		CriticalEdges: criticalEdges,
	}, nil
}

// extractCriticalPathChain traces back from the maximum finish node to the root.
func (a *Analyzer) extractCriticalPathChain(g *model.Graph, totalDuration time.Duration) []*model.Node {
	// Find the end node that finishes at totalDuration and is critical
	var endNode *model.Node
	for _, node := range g.Nodes {
		if node.IsCritical && node.EarliestFinish == totalDuration {
			endNode = node
			break
		}
	}

	if endNode == nil {
		return nil
	}

	// Backtrack
	chain := []*model.Node{endNode}
	curr := endNode

	for {
		preds := g.Predecessors[curr.ID]
		if len(preds) == 0 {
			break
		}

		// Find the predecessor that supplied curr.EarliestStart and is critical
		var bestPred *model.Node
		for _, predID := range preds {
			pred := g.Nodes[predID]
			if pred.IsCritical && pred.EarliestFinish == curr.EarliestStart {
				bestPred = pred
				break
			}
		}

		if bestPred == nil {
			break
		}

		chain = append(chain, bestPred)
		curr = bestPred
	}

	// Reverse chain to be in topological/execution order (Root -> Leaf)
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}

	return chain
}

// topologicalSort returns node IDs in topological order using Kahn's algorithm.
func (a *Analyzer) topologicalSort(g *model.Graph) ([]string, error) {
	inDegree := make(map[string]int)
	for id := range g.Nodes {
		inDegree[id] = len(g.Predecessors[id])
	}

	queue := make([]string, 0)
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	result := make([]string, 0, len(g.Nodes))
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		result = append(result, curr)

		for _, succ := range g.Successors[curr] {
			inDegree[succ]--
			if inDegree[succ] == 0 {
				queue = append(queue, succ)
			}
		}
	}

	if len(result) != len(g.Nodes) {
		return nil, fmt.Errorf("graph has cycles (%d of %d visited)", len(result), len(g.Nodes))
	}

	return result, nil
}
