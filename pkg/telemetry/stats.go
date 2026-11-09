package telemetry

type Stats struct {
	Turns  int
	Steps  int
	LLMMs  int64
	TTFTMs int64
	ToolMs int64
}

type openStepStat struct {
	turn       int
	start      int64
	firstToken int64
}

type StatsCollector struct {
	stats    Stats
	lastTurn *int
	open     *openStepStat
	pending  map[string]int64
}

func NewStatsCollector() *StatsCollector {
	return &StatsCollector{pending: map[string]int64{}}
}

func (c *StatsCollector) StepStart(turn int, timeMs int64) {
	c.open = &openStepStat{turn: turn, start: timeMs}
}

func (c *StatsCollector) FirstToken(turn int, timeMs int64) {
	if c.open == nil || c.open.turn != turn || c.open.firstToken > 0 {
		return
	}
	c.open.firstToken = timeMs
}

func (c *StatsCollector) Message(turn int, timeMs int64) {
	if c.open == nil || c.open.turn != turn {
		return
	}
	if d := timeMs - c.open.start; d > 0 {
		c.stats.LLMMs += d
	}
	if c.open.firstToken > 0 {
		if d := c.open.firstToken - c.open.start; d > 0 {
			c.stats.TTFTMs += d
		}
	}
	c.open = nil
}

func (c *StatsCollector) ToolCall(callID string, timeMs int64) {
	c.pending[callID] = timeMs
}

func (c *StatsCollector) ToolResult(callID string, timeMs int64) {
	dispatched, ok := c.pending[callID]
	if !ok {
		return
	}
	delete(c.pending, callID)
	if d := timeMs - dispatched; d > 0 {
		c.stats.ToolMs += d
	}
}

func (c *StatsCollector) StepEnd(turn int) {
	if c.lastTurn == nil || *c.lastTurn != turn {
		c.stats.Turns++
		t := turn
		c.lastTurn = &t
	}
	c.stats.Steps++
	c.open = nil
}

func (c *StatsCollector) Snapshot() Stats {
	return c.stats
}
