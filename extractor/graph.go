package extractor

import (
	"fmt"
	"strings"
	"time"

	"github.com/jaegertracing/jaeger/model"
)

type Span struct {
	spanID    model.SpanID
	podName   string
	startTime time.Duration
	duration  time.Duration
	children  []*Span
	parent    *Span
}

func (sp *Span) GetPodName() string {
	return sp.podName
}

func (sp *Span) GetDuration() time.Duration {
	return sp.duration
}

type Graph struct {
	traceID     model.TraceID
	root        *Span
	startTime   time.Time
	longestPath *Path
}

type PathNode struct {
	span *Span
	next *PathNode
}

func (p *PathNode) GetSpan() *Span {
	return p.span
}

func (p *PathNode) GetNext() *PathNode {
	return p.next
}

type Path struct {
	head *PathNode
}

func (p *Path) GetHead() *PathNode {
	return p.head
}

func NewGraph(trace *model.Trace) *Graph {
	var graph Graph
	spanMap := make(map[model.SpanID]*Span)

	spans := trace.Spans
	for _, span := range spans {
		spanMap[span.SpanID] = &Span{
			spanID:   span.SpanID,
			podName:  span.Process.Tags[1].VStr,
			duration: span.Duration,
			children: make([]*Span, 0),
			parent:   nil,
		}
		if span.TraceID.String() == span.SpanID.String() {
			graph.traceID = span.TraceID
			graph.startTime = span.StartTime
			graph.root = spanMap[span.SpanID]
		}
	}

	for _, span := range spans {
		spanMap[span.SpanID].startTime = span.StartTime.Sub(graph.startTime)
		ref := span.References[0]
		if ref.RefType == model.ChildOf {
			spanMap[span.SpanID].parent = spanMap[ref.SpanID]
			spanMap[ref.SpanID].children = append(spanMap[ref.SpanID].children, spanMap[span.SpanID])
		}
	}

	graph.longestPath = graph.buildLongestPath()

	return &graph
}

func (g *Graph) buildLongestPath() *Path {
	path := &Path{
		head: &PathNode{
			span: g.root,
			next: nil,
		},
	}

	curr := g.root
	currPathNode := path.head
	for curr.children != nil {
		maxChild := curr
		maxDuration := 0
		for _, child := range curr.children {
			if maxDuration < int(child.duration) {
				maxDuration = int(child.duration)
				maxChild = child
			}
		}
		curr = maxChild
		currPathNode.next = &PathNode{
			span: curr,
			next: nil,
		}
		currPathNode = currPathNode.next
	}
	return path
}

func (g *Graph) PrintLongestPath() {
	if g.longestPath == nil {
		g.longestPath = g.buildLongestPath()
	}
	curr := g.longestPath.head
	for curr != nil {
		span := curr.span
		fmt.Println(span.podName, span.startTime, span.duration)
		curr = curr.next
	}
}

func doPrint(currSpan *Span, level int, printFunc func(int, *Span)) {
	printFunc(level, currSpan)
	for _, child := range currSpan.children {
		doPrint(child, level+1, printFunc)
	}
}

func (g *Graph) PrintGraph() {
	doPrint(g.root, 0, func(level int, span *Span) {
		fmt.Println(strings.Repeat("  ", level), span.podName, span.startTime, span.duration)
	})
}

func (g *Graph) GetLongestPath() *Path {
	return g.longestPath
}
