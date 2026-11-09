//

package tests

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/JopenChen/dsh-go/pkg/terminal"
)

func TestTerminalSpawnReadWait(t *testing.T) {
	ctx := context.Background()
	name, args := terminal.ShellCommand(echoScript("TERM_MARKER_123"))
	backend := terminal.NewBackend(terminal.TerminalConfig{Command: name, Args: args})
	sess, err := backend.Spawn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	if !waitForOutput(t, sess, "TERM_MARKER_123", 3*time.Second) {
		t.Fatalf("did not read expected output TERM_MARKER_123, rollback=%q", sess.ReadString())
	}
	if reason := sess.Wait(); reason != terminal.WaitExited {
		t.Fatalf("wait reason should be exited, got %s", reason)
	}
}

func TestTerminalBoundedScrollback(t *testing.T) {
	ctx := context.Background()
	name, args := terminal.ShellCommand(lineLoopScript("line-", 20))
	backend := terminal.NewBackend(terminal.TerminalConfig{Command: name, Args: args, MaxLines: 5})
	sess, err := backend.Spawn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	_ = sess.Wait()

	if lines := sess.Read(); len(lines) > 5 {
		t.Fatalf("bounded rollback should be <= 5 lines, got %d lines", len(lines))
	}
	if !strings.Contains(strings.Join(sess.Read(), "\n"), "line-20") {
		t.Fatalf("should retain latest line line-20, got %q", sess.ReadString())
	}
}

func TestTerminalExclusive(t *testing.T) {
	ctx := context.Background()
	name, args := sleepScript()
	backend := terminal.NewBackend(terminal.TerminalConfig{Command: name, Args: args})
	sess, err := backend.Spawn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	if _, err := backend.Spawn(ctx); err == nil {
		t.Fatal("spawn while active session exists should be rejected (single agent exclusive)")
	}
}

func TestTerminalInteractiveSendRead(t *testing.T) {
	name, args := interactiveShell()
	if name == "" {
		t.Skip("no interactive shell available on current platform")
	}
	ctx := context.Background()
	backend := terminal.NewBackend(terminal.TerminalConfig{Command: name, Args: args})
	sess, err := backend.Spawn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	if err := sess.SendLine("echo INTERACTIVE_OK"); err != nil {
		t.Fatal(err)
	}
	if !waitForOutput(t, sess, "INTERACTIVE_OK", 4*time.Second) {
		t.Fatalf("did not read interactive output INTERACTIVE_OK, rollback=%q", sess.ReadString())
	}
	_ = sess.SendLine("exit")
}

// ============================================================================

// ============================================================================

func echoScript(text string) string {
	esc := strings.ReplaceAll(text, `"`, `""`)
	if runtime.GOOS == "windows" {
		return "echo " + esc
	}
	return "echo \"" + esc + "\""
}

func lineLoopScript(prefix string, n int) string {
	if runtime.GOOS == "windows" {
		return "for /L %i in (1,1," + itoa(n) + ") do @echo " + prefix + "%i"
	}
	return "i=1; while [ $i -le " + itoa(n) + " ]; do echo " + prefix + "$i; i=$((i+1)); done"
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

func sleepScript() (string, []string) {
	if runtime.GOOS == "windows" {
		return "powershell", []string{"-NoProfile", "-Command", "Start-Sleep -Seconds 1"}
	}
	return "sh", []string{"-c", "sleep 1"}
}

func interactiveShell() (string, []string) {
	if runtime.GOOS == "windows" {

		return "cmd", nil
	}
	return "sh", nil
}

func waitForOutput(t *testing.T, sess *terminal.Session, substr string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if sess.Contains(substr) {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return sess.Contains(substr)
}
