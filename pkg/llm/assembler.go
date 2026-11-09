//

package llm

type BlockAssembler struct {
	curKind  StreamChunkKind
	curBuf   string
	blocks   []ContentBlock
	finished bool
}

func NewBlockAssembler() *BlockAssembler {
	return &BlockAssembler{}
}

func (a *BlockAssembler) flush() {
	if a.curKind == "" {
		return
	}
	if a.curBuf != "" {
		a.blocks = append(a.blocks, ContentBlock{
			Kind: BlockReasoning,
			Text: a.curBuf,
		})
		if a.curKind == ChunkText {
			a.blocks[len(a.blocks)-1].Kind = BlockText
		}
	}
	a.curKind = ""
	a.curBuf = ""
}

func (a *BlockAssembler) Push(c StreamChunk) {
	switch c.Kind {
	case ChunkText:
		if a.curKind != ChunkText {
			a.flush()
			a.curKind = ChunkText
		}
		a.curBuf += c.Text
	case ChunkReasoning:
		if a.curKind != ChunkReasoning {
			a.flush()
			a.curKind = ChunkReasoning
		}
		a.curBuf += c.Reasoning
	case ChunkToolCall:
		a.flush()
		if c.ToolCall != nil {
			a.blocks = append(a.blocks, ToolUse(c.ToolCall))
		}
	case ChunkDone:
		a.flush()
		a.finished = true
	}
}

func (a *BlockAssembler) Blocks() []ContentBlock {
	a.flush()
	out := make([]ContentBlock, len(a.blocks))
	copy(out, a.blocks)
	return out
}

func (a *BlockAssembler) Message() Message {
	return Message{Role: RoleAssistant, Content: a.Blocks()}
}

func (a *BlockAssembler) Finished() bool { return a.finished }
