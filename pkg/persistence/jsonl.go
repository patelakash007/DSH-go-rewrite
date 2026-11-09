//

//

package persistence

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/session"
)

// ============================================================================

//

// ============================================================================

const defaultMarshalBufCap = 4 * 1024

const defaultBufioWriterSize = 32 * 1024

var (
	marshalBufPool = sync.Pool{
		New: func() any { return bytes.NewBuffer(make([]byte, 0, defaultMarshalBufCap)) },
	}

	bufioWriterPool = sync.Pool{
		New: func() any { return bufio.NewWriterSize(io.Discard, defaultBufioWriterSize) },
	}
)

var (
	ioStatPooledBufferHits     atomic.Uint64
	ioStatMarshaledBytes       atomic.Uint64
	ioStatMarshaledEvents      atomic.Uint64
	ioStatHeaderMarshaledBytes atomic.Uint64
)

type JSONLIOStats struct {
	PooledBufferHits     uint64 `json:"pooledBufferHits"`
	MarshaledBytes       uint64 `json:"marshaledBytes"`
	MarshaledEvents      uint64 `json:"marshaledEvents"`
	HeaderMarshaledBytes uint64 `json:"headerMarshaledBytes"`
}

func ReadJSONLIOStats() JSONLIOStats {
	return JSONLIOStats{
		PooledBufferHits:     ioStatPooledBufferHits.Load(),
		MarshaledBytes:       ioStatMarshaledBytes.Load(),
		MarshaledEvents:      ioStatMarshaledEvents.Load(),
		HeaderMarshaledBytes: ioStatHeaderMarshaledBytes.Load(),
	}
}

func ResetJSONLIOStats() {
	ioStatPooledBufferHits.Store(0)
	ioStatMarshaledBytes.Store(0)
	ioStatMarshaledEvents.Store(0)
	ioStatHeaderMarshaledBytes.Store(0)
}

func pooledMarshalEvent(v any) (buf *bytes.Buffer, err error) {
	buf, _ = marshalBufPool.Get().(*bytes.Buffer)
	buf.Reset()
	ioStatPooledBufferHits.Add(1)
	enc := json.NewEncoder(buf)
	if err = enc.Encode(v); err != nil {
		return
	}
	return
}

func releasePooledBuffer(buf *bytes.Buffer) {
	if buf == nil {
		return
	}
	marshalBufPool.Put(buf)
}

func pooledBufioWriter(w io.Writer) *bufio.Writer {
	bw, _ := bufioWriterPool.Get().(*bufio.Writer)
	bw.Reset(w)
	ioStatPooledBufferHits.Add(1)
	return bw
}

func releasePooledWriter(bw *bufio.Writer) error {
	if bw == nil {
		return nil
	}
	err := bw.Flush()

	bw.Reset(io.Discard)
	bufioWriterPool.Put(bw)
	return err
}

// ============================================================================

// ============================================================================

const (
	DefaultShardCount = 16

	DefaultFlushInterval = 100 * time.Millisecond
)

// ============================================================================

// ============================================================================

type Persistence interface {
	Locate(ctx context.Context, id brand.SessionID) (string, error)

	Load(ctx context.Context, id brand.SessionID) (*session.SessionHeader, []session.SessionEvent, error)

	Append(ctx context.Context, id brand.SessionID, ev session.SessionEvent) error

	Flush(ctx context.Context, id brand.SessionID) error

	Snapshot(ctx context.Context, id brand.SessionID) (int, error)

	List(ctx context.Context) ([]brand.SessionID, error)
}

// ============================================================================

// ============================================================================

//

type shardBuf struct {
	mu  sync.Mutex
	buf map[brand.SessionID][]session.SessionEvent

	writeMu sync.Mutex
}

//

//

type JSONLBackend struct {
	dir           string
	batchSize     int
	flushInterval time.Duration

	shards    []*shardBuf
	shardMask uint32

	flushChs []chan struct{}

	closeOnce sync.Once
	closeCh   chan struct{}
	wg        sync.WaitGroup
}

type Option func(*jsonlOptions)

type jsonlOptions struct {
	shardCount    int
	flushInterval time.Duration
}

func WithShardCount(n int) Option {
	return func(o *jsonlOptions) { o.shardCount = n }
}

func WithFlushInterval(d time.Duration) Option {
	return func(o *jsonlOptions) { o.flushInterval = d }
}

//

//

// DefaultFlushInterval。
func NewJSONL(dir string, batchSize int, opts ...Option) (*JSONLBackend, error) {

	o := &jsonlOptions{
		shardCount:    DefaultShardCount,
		flushInterval: DefaultFlushInterval,
	}
	for _, f := range opts {
		f(o)
	}

	shardCount := nextPow2(o.shardCount)
	if shardCount < 1 {
		shardCount = 1
	}
	if batchSize <= 0 {
		batchSize = 1
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	shards := make([]*shardBuf, shardCount)
	flushChs := make([]chan struct{}, shardCount)
	for i := 0; i < shardCount; i++ {
		shards[i] = &shardBuf{buf: map[brand.SessionID][]session.SessionEvent{}}
		flushChs[i] = make(chan struct{}, 1)
	}
	jb := &JSONLBackend{
		dir:           dir,
		batchSize:     batchSize,
		flushInterval: o.flushInterval,
		shards:        shards,
		shardMask:     uint32(shardCount - 1),
		flushChs:      flushChs,
		closeCh:       make(chan struct{}),
	}

	for i := 0; i < shardCount; i++ {
		jb.wg.Add(1)
		go jb.shardWriter(i)
	}
	return jb, nil
}

func nextPow2(n int) int {
	if n <= 1 {
		return 1
	}
	p := 1
	for p < n {
		p <<= 1
	}
	return p
}

func (j *JSONLBackend) shardIndex(id brand.SessionID) int {
	h := fnv.New32a()

	_, _ = h.Write([]byte(id.Raw()))
	return int(h.Sum32() & j.shardMask)
}

func (j *JSONLBackend) shardWriter(idx int) {
	defer j.wg.Done()
	sh := j.shards[idx]
	trigger := j.flushChs[idx]
	ticker := time.NewTicker(j.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-j.closeCh:

			_ = j.flushShard(sh)
			return
		case <-trigger:
			_ = j.flushShard(sh)
		case <-ticker.C:
			_ = j.flushShard(sh)
		}
	}
}

func (j *JSONLBackend) flushShard(sh *shardBuf) error {

	sh.mu.Lock()
	if len(sh.buf) == 0 {
		sh.mu.Unlock()
		return nil
	}
	snapshot := sh.buf
	sh.buf = make(map[brand.SessionID][]session.SessionEvent, len(snapshot))
	sh.mu.Unlock()

	sh.writeMu.Lock()
	defer sh.writeMu.Unlock()
	for id, events := range snapshot {
		if len(events) == 0 {
			continue
		}
		if err := j.appendEventsToFile(id, events); err != nil {

			sh.mu.Lock()
			sh.buf[id] = append(events, sh.buf[id]...)
			sh.mu.Unlock()
			return err
		}
	}
	return nil
}

//

func (j *JSONLBackend) appendEventsToFile(id brand.SessionID, events []session.SessionEvent) error {
	dir := j.sessionDir(id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(j.dataPath(id), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	w := pooledBufioWriter(f)
	defer func() { _ = releasePooledWriter(w) }()
	for _, ev := range events {
		buf, merr := pooledMarshalEvent(ev)
		if merr != nil {
			return merr
		}
		n, werr := w.Write(buf.Bytes())
		ioStatMarshaledBytes.Add(uint64(n))
		ioStatMarshaledEvents.Add(1)
		releasePooledBuffer(buf)
		if werr != nil {
			return werr
		}
	}

	return nil
}

func (j *JSONLBackend) Close() error {
	j.closeOnce.Do(func() {
		close(j.closeCh)
	})
	j.wg.Wait()
	return nil
}

func (j *JSONLBackend) ShardCount() int { return len(j.shards) }

// ============================================================================

// ============================================================================

func (j *JSONLBackend) sessionDir(id brand.SessionID) string {
	return filepath.Join(j.dir, id.Raw())
}

func (j *JSONLBackend) headerPath(id brand.SessionID) string {
	return filepath.Join(j.sessionDir(id), "header.json")
}
func (j *JSONLBackend) dataPath(id brand.SessionID) string {
	return filepath.Join(j.sessionDir(id), "events.jsonl")
}

// ============================================================================

// ============================================================================

func (j *JSONLBackend) Locate(ctx context.Context, id brand.SessionID) (string, error) {
	return j.sessionDir(id), nil
}

func (j *JSONLBackend) SaveHeader(ctx context.Context, h *session.SessionHeader) error {
	dir := j.sessionDir(h.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := h.Marshal()
	if err != nil {
		return err
	}
	return os.WriteFile(j.headerPath(h.ID), data, 0o644)
}

func (j *JSONLBackend) Append(ctx context.Context, id brand.SessionID, ev session.SessionEvent) error {
	idx := j.shardIndex(id)
	sh := j.shards[idx]

	sh.mu.Lock()
	sh.buf[id] = append(sh.buf[id], ev)
	need := len(sh.buf[id]) >= j.batchSize
	sh.mu.Unlock()

	if need {

		select {
		case j.flushChs[idx] <- struct{}{}:
		default:
		}
	}
	return nil
}

//

func (j *JSONLBackend) Flush(ctx context.Context, id brand.SessionID) error {
	idx := j.shardIndex(id)
	sh := j.shards[idx]

	sh.mu.Lock()
	events := sh.buf[id]
	delete(sh.buf, id)
	sh.mu.Unlock()

	if len(events) == 0 {
		return nil
	}
	sh.writeMu.Lock()
	defer sh.writeMu.Unlock()
	return j.appendEventsToFile(id, events)
}

func (j *JSONLBackend) FlushAll(ctx context.Context) error {
	for _, sh := range j.shards {
		sh.mu.Lock()
		snapshot := sh.buf
		sh.buf = make(map[brand.SessionID][]session.SessionEvent, len(snapshot))
		sh.mu.Unlock()

		sh.writeMu.Lock()
		writeErr := error(nil)
		for id, events := range snapshot {
			if len(events) == 0 {
				continue
			}
			if err := j.appendEventsToFile(id, events); err != nil {
				writeErr = err

				sh.mu.Lock()
				sh.buf[id] = append(events, sh.buf[id]...)
				sh.mu.Unlock()
				break
			}
		}
		sh.writeMu.Unlock()
		if writeErr != nil {
			return writeErr
		}
	}
	return nil
}

func (j *JSONLBackend) Load(ctx context.Context, id brand.SessionID) (*session.SessionHeader, []session.SessionEvent, error) {

	hData, err := os.ReadFile(j.headerPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("persistence: session %q not found", id.Raw())
		}
		return nil, nil, err
	}
	header, err := session.UnmarshalSessionHeader(hData)
	if err != nil {
		return nil, nil, err
	}

	f, err := os.Open(j.dataPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return header, nil, nil
		}
		return nil, nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	var events []session.SessionEvent
	for scanner.Scan() {
		var ev session.SessionEvent
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			return nil, nil, fmt.Errorf("persistence: corrupt line: %w", err)
		}
		events = append(events, ev)
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}

	if repaired := repairOrphanTurn(&events); repaired > 0 {
		if err := j.rewrite(header, events); err != nil {
			return nil, nil, err
		}
	}
	return header, events, nil
}

//

func (j *JSONLBackend) rewrite(header *session.SessionHeader, events []session.SessionEvent) error {
	dir := j.sessionDir(header.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	hData, err := header.Marshal()
	if err != nil {
		return err
	}
	hBuf, _ := marshalBufPool.Get().(*bytes.Buffer)
	hBuf.Reset()
	ioStatPooledBufferHits.Add(1)
	_, _ = hBuf.Write(hData)

	hBytes := append([]byte(nil), hBuf.Bytes()...)
	releasePooledBuffer(hBuf)
	ioStatHeaderMarshaledBytes.Add(uint64(len(hBytes)))
	if err := os.WriteFile(j.headerPath(header.ID), hBytes, 0o644); err != nil {
		return err
	}

	f, err := os.Create(j.dataPath(header.ID))
	if err != nil {
		return err
	}
	defer f.Close()
	w := pooledBufioWriter(f)
	defer func() { _ = releasePooledWriter(w) }()
	for _, ev := range events {
		buf, merr := pooledMarshalEvent(ev)
		if merr != nil {
			return merr
		}
		n, werr := w.Write(buf.Bytes())
		ioStatMarshaledBytes.Add(uint64(n))
		ioStatMarshaledEvents.Add(1)
		releasePooledBuffer(buf)
		if werr != nil {
			return werr
		}
	}
	return nil
}

func (j *JSONLBackend) Snapshot(ctx context.Context, id brand.SessionID) (int, error) {
	if err := j.Flush(ctx, id); err != nil {
		return 0, err
	}
	_, events, err := j.Load(ctx, id)
	if err != nil {
		return 0, err
	}
	return len(events), nil
}

func (j *JSONLBackend) List(ctx context.Context) ([]brand.SessionID, error) {
	entries, err := os.ReadDir(j.dir)
	if err != nil {
		return nil, err
	}
	var out []brand.SessionID
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, brand.NewSessionID(e.Name()))
		}
	}
	return out, nil
}

// ============================================================================

// ============================================================================

func repairOrphanTurn(events *[]session.SessionEvent) int {
	if len(*events) == 0 {
		return 0
	}
	turnOpen := false
	stepOpen := false
	var openTurn uint64
	var openStep uint64

	var pending []string
	hasResult := map[string]bool{}
	for _, ev := range *events {
		switch ev.Type {
		case session.EventTurnStart:
			turnOpen, stepOpen = true, false
			pending = nil
			if td, ok := ev.Data.(session.TurnStartData); ok {
				openTurn = td.Turn
			}
		case session.EventTurnEnd:
			turnOpen, stepOpen = false, false
			pending = nil
		case session.EventStepStart:
			stepOpen = true
			if sd, ok := ev.Data.(session.StepStartData); ok {
				openStep = sd.Step
			}
		case session.EventStepEnd:
			stepOpen = false
		case session.EventAssistantMessage:
			if am, ok := ev.Data.(session.AssistantMessageData); ok {
				for _, id := range am.ToolCallIDs {
					if !hasResult[id] {
						pending = append(pending, id)
					}
				}
			}
		case session.EventToolResult:
			if tr, ok := ev.Data.(session.ToolResultData); ok {
				hasResult[tr.CallID.String()] = true
			}
		}
	}
	if !turnOpen {
		return 0
	}
	last := (*events)[len(*events)-1]
	seq := last.Seq
	time := last.Time
	add := func(ev session.SessionEvent) {
		seq++
		ev.Seq, ev.Time = seq, time
		*events = append(*events, ev)
	}
	n := 0

	for _, id := range pending {
		if hasResult[id] {
			continue
		}
		add(session.SessionEvent{
			Type: session.EventToolResult,
			Data: session.ToolResultData{
				CallID:  brand.NewToolCallID(id),
				IsError: true,
				Output:  "Tool call was interrupted before recording results; result unknown. If still needed, retry after confirming external state.",
			},
		})
		n++
	}

	if stepOpen {
		add(session.SessionEvent{
			Type: session.EventStepEnd,
			Data: session.StepEndData{Turn: openTurn, Step: openStep},
		})
		n++
	}

	add(session.SessionEvent{
		Type: session.EventTurnEnd,
		Data: session.TurnEndData{Turn: openTurn, Reason: session.ReasonInterrupted},
	})
	n++
	return n
}
