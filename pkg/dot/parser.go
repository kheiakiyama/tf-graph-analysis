package dot

import (
	"bufio"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/kheiakiyama/tf-graph-analysis/pkg/model"
)

var (
	// Matches edge: "nodeA" -> "nodeB" [attrs] (handles escaped quotes)
	edgeRegex = regexp.MustCompile(`^\s*"((?:\\.|[^"\\])+)"\s*->\s*"((?:\\.|[^"\\])+)"`)
	// Matches node: "nodeA" [label = "...", shape = "..."] (handles escaped quotes)
	nodeRegex = regexp.MustCompile(`^\s*"((?:\\.|[^"\\])+)"\s*\[(.*)\]`)
	// Matches label attribute: label = "..." (handles escaped quotes)
	labelRegex = regexp.MustCompile(`label\s*=\s*"((?:\\.|[^"\\])*)"`)
)

// Parser parses Terraform DOT output into model.Graph.
type Parser struct {
	FilterResourcesOnly bool // If true, exclude meta/provider nodes where possible
}

// NewParser creates a new DOT parser.
func NewParser(filterResourcesOnly bool) *Parser {
	return &Parser{
		FilterResourcesOnly: filterResourcesOnly,
	}
}

// ParseFile parses a DOT file from the specified path.
func (p *Parser) ParseFile(path string) (*model.Graph, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return p.Parse(f)
}

// Parse parses DOT content from an io.Reader.
func (p *Parser) Parse(r io.Reader) (*model.Graph, error) {
	scanner := bufio.NewScanner(r)
	g := model.NewGraph()

	// Temporary storage to collect raw edges: source -> target
	// Note: in Terraform DOT, source depends on target (target must finish before source can run).
	type rawEdge struct {
		from string // dependent
		to   string // prerequisite
	}
	var rawEdges []rawEdge

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#") {
			continue
		}

		// Check for edge
		if edgeMatches := edgeRegex.FindStringSubmatch(line); edgeMatches != nil {
			source := edgeMatches[1]
			target := edgeMatches[2]
			rawEdges = append(rawEdges, rawEdge{from: source, to: target})

			// Ensure nodes exist
			p.ensureNode(g, source)
			p.ensureNode(g, target)
			continue
		}

		// Check for node definition with attributes
		if nodeMatches := nodeRegex.FindStringSubmatch(line); nodeMatches != nil {
			nodeID := nodeMatches[1]
			attrs := nodeMatches[2]

			node := p.ensureNode(g, nodeID)
			if labelMatch := labelRegex.FindStringSubmatch(attrs); labelMatch != nil {
				label := strings.ReplaceAll(labelMatch[1], `\"`, `"`)
				cleaned, isRes := CleanResourceName(label)
				node.Name = cleaned
				if isRes {
					node.IsResource = true
				}
			}
			continue
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Add execution edges:
	// In Terraform DOT: `A -> B` means "A depends on B".
	// Therefore, the execution flow is B (predecessor) -> A (successor).
	for _, re := range rawEdges {
		predecessor := re.to
		successor := re.from

		if p.FilterResourcesOnly {
			// If filtering, only add edge if both are resources (or keep all for accurate graph)
			nodePred := g.Nodes[predecessor]
			nodeSucc := g.Nodes[successor]
			if nodePred != nil && nodeSucc != nil && (!nodePred.IsResource || !nodeSucc.IsResource) {
				// For resources-only mode, we still want dependencies preserved,
				// but let's connect all nodes by default unless pruned.
			}
		}

		g.AddEdge(predecessor, successor)
	}

	return g, nil
}

func (p *Parser) ensureNode(g *model.Graph, id string) *model.Node {
	if node, exists := g.Nodes[id]; exists {
		return node
	}
	cleanName, isRes := CleanResourceName(id)
	nodeType := "meta"
	if isRes {
		nodeType = "resource"
	} else if strings.Contains(id, "provider[") {
		nodeType = "provider"
	}

	node := &model.Node{
		ID:         id,
		Name:       cleanName,
		Type:       nodeType,
		IsResource: isRes,
	}
	g.AddNode(node)
	return node
}

// CleanResourceName strips `[root]`, `(expand)`, `(close)`, etc. and returns a clean resource name.
func CleanResourceName(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	// Strip "[root] " prefix
	s = strings.TrimPrefix(s, "[root] ")

	// Strip " (expand)" or " (close)" suffix
	s = strings.TrimSuffix(s, " (expand)")
	s = strings.TrimSuffix(s, " (close)")

	// Meta nodes
	if s == "root" || strings.HasPrefix(s, "meta.") || strings.HasPrefix(s, "var.") || strings.HasPrefix(s, "local.") || strings.HasPrefix(s, "output.") {
		return s, false
	}
	if strings.HasPrefix(s, "provider[") {
		cleaned := strings.ReplaceAll(s, `\"`, `"`)
		return cleaned, false
	}

	// Remove any module.<name>. prefixes to examine the tail
	tail := s
	for strings.HasPrefix(tail, "module.") {
		idx := strings.Index(tail[7:], ".")
		if idx == -1 {
			// Just "module.foo" - this is a module call node, not a resource
			return s, false
		}
		tail = tail[7+idx+1:]
	}

	// Now tail is either:
	// - "var.foo" or "output.bar" -> not a resource
	// - "data.<type>.<name>" -> data resource
	// - "<type>.<name>" -> managed resource
	if strings.HasPrefix(tail, "var.") || strings.HasPrefix(tail, "output.") || strings.HasPrefix(tail, "local.") || strings.HasPrefix(tail, "meta.") {
		return s, false
	}

	parts := strings.Split(tail, ".")
	if len(parts) >= 2 {
		return s, true
	}

	return s, false
}
