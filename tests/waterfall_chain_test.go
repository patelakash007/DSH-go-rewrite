package tests

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/waterfall"
)

type testPayload struct {
	seq     []string
	msg     string
	blocked bool
}

func TestWaterfallOrdering(t *testing.T) {
	p := &testPayload{seq: []string{}}

	chain := waterfall.New[testPayload](
		func(p *testPayload, next waterfall.NextFunc) error {
			p.seq = append(p.seq, "A-in")
			err := next()
			p.seq = append(p.seq, "A-out")
			return err
		},
		func(p *testPayload, next waterfall.NextFunc) error {
			p.seq = append(p.seq, "B-in")
			err := next()
			p.seq = append(p.seq, "B-out")
			return err
		},
		func(p *testPayload, next waterfall.NextFunc) error {
			p.seq = append(p.seq, "C-in")
			err := next()
			p.seq = append(p.seq, "C-out")
			return err
		},
	)

	if err := chain.Run(context.Background(), p); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	want := []string{"A-in", "B-in", "C-in", "C-out", "B-out", "A-out"}
	if strings.Join(p.seq, ",") != strings.Join(want, ",") {
		t.Fatalf("call order = %v, want %v", p.seq, want)
	}
}

func TestWaterfallShortCircuit(t *testing.T) {
	var entered int32
	p := &testPayload{seq: []string{}}

	chain := waterfall.New[testPayload](
		func(p *testPayload, next waterfall.NextFunc) error {
			p.seq = append(p.seq, "guard")

			return nil
		},
		func(p *testPayload, next waterfall.NextFunc) error {
			atomic.AddInt32(&entered, 1)
			return next()
		},
	)

	if err := chain.Run(context.Background(), p); err != nil {
		t.Fatalf("Run in short-circuit scenario should return nil, got %v", err)
	}
	if entered != 0 {
		t.Fatalf("subsequent middleware should not execute after short-circuit, entered = %d", entered)
	}
	if len(p.seq) != 1 {
		t.Fatalf("only first middleware should enter, seq = %v", p.seq)
	}
}

func TestWaterfallPayloadRewrite(t *testing.T) {
	p := &testPayload{msg: "initial"}

	chain := waterfall.New[testPayload](
		func(p *testPayload, next waterfall.NextFunc) error {
			p.msg += "+m1"
			return next()
		},
		func(p *testPayload, next waterfall.NextFunc) error {
			p.msg += "+m2"
			return next()
		},
		func(p *testPayload, next waterfall.NextFunc) error {
			p.msg += "+m3"
			return next()
		},
	)

	if err := chain.Run(context.Background(), p); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if p.msg != "initial+m1+m2+m3" {
		t.Fatalf("rewritten result = %q, want %q", p.msg, "initial+m1+m2+m3")
	}
}

func TestWaterfallErrorPropagation(t *testing.T) {
	var after int32
	boom := errors.New("boom")
	p := &testPayload{}

	chain := waterfall.New[testPayload](
		func(p *testPayload, next waterfall.NextFunc) error {
			return next()
		},
		func(p *testPayload, next waterfall.NextFunc) error {
			return boom
		},
		func(p *testPayload, next waterfall.NextFunc) error {
			atomic.AddInt32(&after, 1)
			return next()
		},
	)

	err := chain.Run(context.Background(), p)
	if !errors.Is(err, boom) {
		t.Fatalf("should propagate boom, got %v", err)
	}
	if after != 0 {
		t.Fatalf("subsequent middleware should not execute after error, after = %d", after)
	}
}

func TestWaterfallNextSingleCall(t *testing.T) {
	p := &testPayload{}

	chain := waterfall.New[testPayload](
		func(p *testPayload, next waterfall.NextFunc) error {
			_ = next()

			_ = next()
			return nil
		},
	)

	defer func() {
		if recover() == nil {
			t.Fatal("duplicate next() call should panic")
		}
	}()
	_ = chain.Run(context.Background(), p)
}

func TestWaterfallDownstreamErrorToUpstream(t *testing.T) {
	p := &testPayload{}

	chain := waterfall.New[testPayload](
		func(p *testPayload, next waterfall.NextFunc) error {
			err := next()
			if err != nil {

				return errors.New("wrapped: " + err.Error())
			}
			return nil
		},
		func(p *testPayload, next waterfall.NextFunc) error {
			return errors.New("inner failure")
		},
	)

	err := chain.Run(context.Background(), p)
	if err == nil || !strings.Contains(err.Error(), "wrapped:") {
		t.Fatalf("should return rewritten error, got %v", err)
	}
}

func TestWaterfallEmptyChain(t *testing.T) {
	chain := waterfall.New[testPayload]()
	if err := chain.Run(context.Background(), &testPayload{}); err != nil {
		t.Fatalf("empty chain should return nil, got %v", err)
	}
}

func TestWaterfallTypedPayloads(t *testing.T) {
	type request struct{ url string }
	type approval struct{ ok bool }

	reqChain := waterfall.New[request](
		func(r *request, next waterfall.NextFunc) error {
			r.url = "https://example.com"
			return next()
		},
	)
	apprChain := waterfall.New[approval](
		func(a *approval, next waterfall.NextFunc) error {
			a.ok = true
			return next()
		},
	)

	req := &request{}
	appr := &approval{}
	_ = reqChain.Run(context.Background(), req)
	_ = apprChain.Run(context.Background(), appr)

	if req.url != "https://example.com" || !appr.ok {
		t.Fatalf("two chains of different types should work independently: req=%+v appr=%+v", req, appr)
	}
}
