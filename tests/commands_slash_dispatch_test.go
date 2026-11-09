package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/commands"
	"github.com/JopenChen/dsh-go/pkg/session"
)

func TestCommandsSlashDispatch(t *testing.T) {
	reg := commands.NewRegistry()
	sl := session.NewSessionLog(brand.NewSessionID("cmd_1"))

	out, err := reg.Dispatch(context.Background(), "/plan off", sl)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	if !strings.Contains(out, "off") {
		t.Fatalf("output should contain off: %q", out)
	}

	hasRun, hasDone, hasPlanMode := false, false, false
	for _, ev := range sl.Events() {
		switch ev.Type {
		case session.EventCommandRun:
			hasRun = true
		case session.EventCommandDone:
			hasDone = true
		case session.EventPlanMode:
			hasPlanMode = true
		}
	}
	if !hasRun || !hasDone || !hasPlanMode {
		t.Fatalf("should write command/run + command/done + plan/mode: run=%v done=%v plan=%v", hasRun, hasDone, hasPlanMode)
	}

	for _, ev := range sl.Events() {
		if ev.Type == session.EventUserMessage {
			t.Fatal("/plan should not produce user/message event")
		}
	}
}

func TestCommandsUnknown(t *testing.T) {
	reg := commands.NewRegistry()
	sl := session.NewSessionLog(brand.NewSessionID("cmd_2"))

	_, err := reg.Dispatch(context.Background(), "/nonexistent x", sl)
	if err == nil {
		t.Fatal("unknown command should return error")
	}
	if _, ok := err.(commands.ErrUnknownCommand); !ok {
		t.Fatalf("should be ErrUnknownCommand, got %T", err)
	}
}

func TestCommandsGoal(t *testing.T) {
	reg := commands.NewRegistry()
	sl := session.NewSessionLog(brand.NewSessionID("cmd_3"))

	if _, err := reg.Dispatch(context.Background(), "/goal complete task", sl); err != nil {
		t.Fatalf("/goal failed: %v", err)
	}
	if _, err := reg.Dispatch(context.Background(), "/goal continue", sl); err != nil {
		t.Fatalf("/goal second attempt failed: %v", err)
	}

	fold := session.FoldGoalChange(sl.Events())
	if !fold.Present {
		t.Fatal("should fold to goal state")
	}
	if fold.Revision != 2 {
		t.Fatalf("revision should increase monotonically to 2: %d", fold.Revision)
	}
	if fold.Description != "continue" {
		t.Fatalf("description should be latest continue: %q", fold.Description)
	}
}

func TestCommandsListRegister(t *testing.T) {
	reg := commands.NewRegistry()
	names := reg.List()
	if len(names) < 2 {
		t.Fatalf("built-in commands should at least have plan/goal: %v", names)
	}

	if names[0] != "goal" || names[1] != "plan" {
		t.Fatalf("commands should be lexicographical: %v", names)
	}
}
