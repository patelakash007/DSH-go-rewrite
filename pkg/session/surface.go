package session

import "encoding/json"

type SurfaceOpKind string

const (
	SurfaceAppend SurfaceOpKind = "append"

	SurfaceReplace SurfaceOpKind = "replace"
)

type SurfaceOp struct {
	Op    SurfaceOpKind   `json:"op"`
	Start uint64          `json:"start,omitempty"`
	End   uint64          `json:"end,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

func NewAppendOp() *SurfaceOp {
	return &SurfaceOp{Op: SurfaceAppend}
}

func NewReplaceOp(start, end uint64, data json.RawMessage) *SurfaceOp {
	return &SurfaceOp{Op: SurfaceReplace, Start: start, End: end, Data: data}
}

// ============================================================================

// ============================================================================

type SurfaceNode struct {
	Seq uint64

	Replaced bool

	Event SessionEvent
}

type ReplaceRange struct {
	Start uint64 `json:"start"`
	End   uint64 `json:"end"`
}

//

func FoldSurface(events []SessionEvent) (nodes []SurfaceNode, replacements []ReplaceRange) {
	hidden := map[uint64]bool{}
	for _, ev := range events {
		if ev.SurfaceOp != nil && ev.SurfaceOp.Op == SurfaceReplace {
			replacements = append(replacements, ReplaceRange{Start: ev.SurfaceOp.Start, End: ev.SurfaceOp.End})
			for s := ev.SurfaceOp.Start; s <= ev.SurfaceOp.End && s <= ev.Seq; s++ {
				hidden[s] = true
			}
		}
	}
	for _, ev := range events {
		if hidden[ev.Seq] {
			continue
		}
		replaced := ev.SurfaceOp != nil && ev.SurfaceOp.Op == SurfaceReplace
		nodes = append(nodes, SurfaceNode{Seq: ev.Seq, Replaced: replaced, Event: ev})
	}
	return nodes, replacements
}
