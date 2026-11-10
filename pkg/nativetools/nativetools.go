// Package nativetools implements the first-party tool set that runs directly
// in the Android Go engine. It intentionally relies only on the app-owned
// workspace and Android's system shell; it never assumes Ubuntu, PRoot,
// Termux, Node.js, bash, npm, or a writable system image.
package nativetools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/JopenChen/dsh-go/pkg/nativefs"
	"github.com/JopenChen/dsh-go/pkg/tools"
)

const (
	maxToolOutput = 1 << 20
	maxSearchFile = 2 << 20
	maxSearchWalk = 10_000
)

type BackgroundRunner func(ctx context.Context, command, cwd string, timeout time.Duration) (map[string]any, error)

type Registry struct {
	fs  *nativefs.Service
	cwd func() string

	background BackgroundRunner

	tools  []*tools.Tool
	byName map[string]*tools.Tool
}

func New(files *nativefs.Service, cwd func() string) *Registry {
	r := &Registry{fs: files, cwd: cwd, byName: map[string]*tools.Tool{}}
	r.register(
		&tools.Tool{Name: "read_file", Description: "Read a UTF-8 text file from the native workspace", Schema: objectSchema(map[string]*tools.JsonSchemaNode{
			"path":   stringSchema("File path, relative to the workspace"),
			"offset": integerSchema("First line (1-based)"),
			"limit":  integerSchema("Maximum number of lines"),
		}, "path"), Execute: r.readFile},
		&tools.Tool{Name: "list_files", Description: "List one directory in the native workspace", Schema: objectSchema(map[string]*tools.JsonSchemaNode{
			"path": stringSchema("Directory path, relative to the workspace"),
		}, "path"), Execute: r.listFiles},
		&tools.Tool{Name: "write_file", Description: "Atomically create or replace a UTF-8 text file in the native workspace", Schema: objectSchema(map[string]*tools.JsonSchemaNode{
			"path":    stringSchema("File path, relative to the workspace"),
			"content": stringSchema("Complete replacement text"),
		}, "path", "content"), Execute: r.writeFile},
		&tools.Tool{Name: "edit_file", Description: "Replace exact text in a UTF-8 workspace file", Schema: objectSchema(map[string]*tools.JsonSchemaNode{
			"path":       stringSchema("File path, relative to the workspace"),
			"oldText":    stringSchema("Exact text to replace"),
			"newText":    stringSchema("Replacement text"),
			"replaceAll": {Type: tools.TypeBoolean, Description: "Replace every match"},
		}, "path", "oldText", "newText"), Execute: r.editFile},
		&tools.Tool{Name: "search_files", Description: "Search UTF-8 workspace files for literal text", Schema: objectSchema(map[string]*tools.JsonSchemaNode{
			"query":      stringSchema("Literal text to find"),
			"path":       stringSchema("Directory or file to search; defaults to workspace root"),
			"maxResults": integerSchema("Maximum matches (1-200)"),
		}, "query"), Execute: r.searchFiles},
		&tools.Tool{Name: "shell", Description: "Run a command with Android's native system shell in the workspace", Schema: objectSchema(map[string]*tools.JsonSchemaNode{
			"command":   stringSchema("Shell command"),
			"cwd":       stringSchema("Working directory relative to the workspace"),
			"timeoutMs": integerSchema("Timeout in milliseconds (maximum 120000)"),
		}, "command"), TimeoutMs: 120_000, Execute: r.shell},
		&tools.Tool{Name: "shell_background", Description: "Start a long-running command in the background and return its job id", Schema: objectSchema(map[string]*tools.JsonSchemaNode{
			"command":   stringSchema("Shell command"),
			"cwd":       stringSchema("Working directory relative to the workspace"),
			"timeoutMs": integerSchema("Optional timeout in milliseconds (maximum 3600000)"),
		}, "command"), Execute: r.shellBackground},
	)
	return r
}

func (r *Registry) SetBackgroundRunner(runner BackgroundRunner) {
	r.background = runner
}

func (r *Registry) register(definitions ...*tools.Tool) {
	for _, definition := range definitions {
		if definition == nil || definition.Name == "" {
			continue
		}
		r.tools = append(r.tools, definition)
		r.byName[definition.Name] = definition
	}
}

func (r *Registry) Tools() []*tools.Tool {
	out := make([]*tools.Tool, len(r.tools))
	copy(out, r.tools)
	return out
}

func (r *Registry) Lookup(name string) *tools.Tool { return r.byName[name] }

func IsDangerous(name string) bool {
	switch name {
	case "write_file", "edit_file", "shell":
		return true
	default:
		return false
	}
}

func (r *Registry) currentCwd() string {
	if r.cwd != nil {
		if cwd := r.cwd(); cwd != "" {
			return cwd
		}
	}
	return r.fs.Root()
}

func (r *Registry) readFile(_ context.Context, input map[string]any) (any, error) {
	path := textArg(input, "path")
	offset := intArg(input, "offset", 1)
	limit := intArg(input, "limit", 500)
	page, err := r.fs.ReadText(r.currentCwd(), path, offset, limit, nativefs.DefaultMaxTextBytes)
	if err != nil {
		return nil, err
	}
	return page, nil
}

func (r *Registry) listFiles(_ context.Context, input map[string]any) (any, error) {
	path := textArg(input, "path")
	if path == "" {
		path = "."
	}
	return r.fs.List(r.currentCwd(), path, nativefs.DefaultMaxEntries)
}

func (r *Registry) writeFile(_ context.Context, input map[string]any) (any, error) {
	path := textArg(input, "path")
	content, ok := input["content"].(string)
	if path == "" || !ok {
		return nil, errors.New("write_file requires path and content")
	}
	stat, err := r.fs.WriteText(r.currentCwd(), path, content)
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "path": path, "version": stat.Version, "bytes": stat.Bytes}, nil
}

func (r *Registry) editFile(_ context.Context, input map[string]any) (any, error) {
	path := textArg(input, "path")
	oldText, oldOK := input["oldText"].(string)
	newText, newOK := input["newText"].(string)
	if path == "" || !oldOK || !newOK {
		return nil, errors.New("edit_file requires path, oldText, and newText")
	}
	stat, err := r.fs.EditText(r.currentCwd(), path, oldText, newText, boolArg(input, "replaceAll"))
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "path": path, "version": stat.Version, "bytes": stat.Bytes}, nil
}

type SearchMatch struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
	Text   string `json:"text"`
}

func (r *Registry) searchFiles(ctx context.Context, input map[string]any) (any, error) {
	query := textArg(input, "query")
	if query == "" {
		return nil, errors.New("search_files requires a non-empty query")
	}
	path := textArg(input, "path")
	if path == "" {
		path = "."
	}
	maxResults := intArg(input, "maxResults", 100)
	if maxResults < 1 {
		maxResults = 1
	}
	if maxResults > 200 {
		maxResults = 200
	}
	start, err := r.fs.Resolve(r.currentCwd(), path)
	if err != nil {
		return nil, err
	}
	matches := make([]SearchMatch, 0, min(maxResults, 32))
	walked := 0
	truncated := false
	err = filepath.WalkDir(start, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if entry.IsDir() {
			if filePath != start && (entry.Name() == ".git" || entry.Name() == ".dsh" || entry.Name() == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		walked++
		if walked > maxSearchWalk {
			truncated = true
			return io.EOF
		}
		info, infoErr := entry.Info()
		if infoErr != nil || !info.Mode().IsRegular() || info.Size() > maxSearchFile {
			return nil
		}
		file, openErr := os.Open(filePath)
		if openErr != nil {
			return nil
		}
		defer file.Close()
		scanner := bufio.NewScanner(io.LimitReader(file, maxSearchFile+1))
		scanner.Buffer(make([]byte, 64<<10), maxSearchFile)
		line := 0
		for scanner.Scan() {
			line++
			text := scanner.Text()
			if !utf8.ValidString(text) || strings.IndexByte(text, 0) >= 0 {
				return nil
			}
			from := 0
			for {
				index := strings.Index(text[from:], query)
				if index < 0 {
					break
				}
				column := from + index
				rel, _ := filepath.Rel(r.fs.Root(), filePath)
				matches = append(matches, SearchMatch{Path: filepath.ToSlash(rel), Line: line, Column: column + 1, Text: boundedLine(text)})
				if len(matches) >= maxResults {
					truncated = true
					return io.EOF
				}
				from = column + len(query)
			}
		}
		return nil
	})
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Path == matches[j].Path {
			if matches[i].Line == matches[j].Line {
				return matches[i].Column < matches[j].Column
			}
			return matches[i].Line < matches[j].Line
		}
		return matches[i].Path < matches[j].Path
	})
	return map[string]any{"matches": matches, "truncated": truncated, "filesScanned": walked}, nil
}

func (r *Registry) shellBackground(ctx context.Context, input map[string]any) (any, error) {
	if r.background == nil {
		return nil, errors.New("background jobs are unavailable in this engine")
	}
	command := textArg(input, "command")
	if strings.TrimSpace(command) == "" {
		return nil, errors.New("shell_background requires a command")
	}
	cwdArg := textArg(input, "cwd")
	if cwdArg == "" {
		cwdArg = "."
	}
	cwd, err := r.fs.Resolve(r.currentCwd(), cwdArg)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(cwd)
	if err != nil || !info.IsDir() {
		return nil, &nativefs.Error{Code: nativefs.ErrNotDir, Path: cwdArg, Err: err}
	}
	timeoutMs := intArg(input, "timeoutMs", 0)
	if timeoutMs < 0 {
		timeoutMs = 0
	}
	if timeoutMs > 3_600_000 {
		timeoutMs = 3_600_000
	}
	return r.background(ctx, command, cwd, time.Duration(timeoutMs)*time.Millisecond)
}

func (r *Registry) shell(ctx context.Context, input map[string]any) (any, error) {
	command := textArg(input, "command")
	if strings.TrimSpace(command) == "" {
		return nil, errors.New("shell requires a command")
	}
	cwdArg := textArg(input, "cwd")
	if cwdArg == "" {
		cwdArg = "."
	}
	cwd, err := r.fs.Resolve(r.currentCwd(), cwdArg)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(cwd)
	if err != nil || !info.IsDir() {
		return nil, &nativefs.Error{Code: nativefs.ErrNotDir, Path: cwdArg, Err: err}
	}
	timeoutMs := intArg(input, "timeoutMs", 30_000)
	if timeoutMs < 1 {
		timeoutMs = 30_000
	}
	if timeoutMs > 120_000 {
		timeoutMs = 120_000
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()
	shell, args := systemShell(command)
	cmd := exec.CommandContext(runCtx, shell, args...)
	cmd.Dir = cwd
	cmd.Env = nativeEnvironment(r.fs.Root())
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	output := &limitedBuffer{limit: maxToolOutput}
	cmd.Stdout = output
	cmd.Stderr = output
	started := time.Now()
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var waitErr error
	select {
	case waitErr = <-done:
	case <-runCtx.Done():
		killProcessTree(cmd)
		waitErr = <-done
	}
	exitCode := 0
	if waitErr != nil {
		var exit *exec.ExitError
		if errors.As(waitErr, &exit) {
			exitCode = exit.ExitCode()
		} else if runCtx.Err() == nil {
			return nil, waitErr
		}
	}
	return map[string]any{
		"output": output.String(), "truncated": output.truncated, "exitCode": exitCode,
		"timedOut":   errors.Is(runCtx.Err(), context.DeadlineExceeded),
		"durationMs": time.Since(started).Milliseconds(),
	}, nil
}

func systemShell(command string) (string, []string) {
	if override := os.Getenv("DSH_SYSTEM_SHELL"); override != "" {
		return override, []string{"-c", command}
	}
	if runtime.GOOS == "android" {
		return "/system/bin/sh", []string{"-c", command}
	}
	if path, err := exec.LookPath("bash"); err == nil {
		return path, []string{"-c", command}
	}
	return "/bin/sh", []string{"-c", command}
}

func nativeEnvironment(home string) []string {
	path := os.Getenv("PATH")
	if runtime.GOOS == "android" || path == "" {
		path = "/system/bin:/system/xbin"
	}
	tmp := os.Getenv("TMPDIR")
	if tmp == "" {
		tmp = os.TempDir()
	}
	return []string{"HOME=" + home, "PATH=" + path, "TMPDIR=" + tmp, "LANG=C.UTF-8"}
}

func killProcessTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if runtime.GOOS != "windows" {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	_ = cmd.Process.Kill()
}

type limitedBuffer struct {
	mu        sync.Mutex
	data      bytes.Buffer
	limit     int
	truncated bool
}

func (b *limitedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	original := len(data)
	remaining := b.limit - b.data.Len()
	if remaining > 0 {
		if len(data) > remaining {
			data = data[:remaining]
			b.truncated = true
		}
		_, _ = b.data.Write(data)
	} else {
		b.truncated = true
	}
	return original, nil
}

func (b *limitedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data.String()
}

func textArg(input map[string]any, name string) string {
	value, _ := input[name].(string)
	return value
}

func boolArg(input map[string]any, name string) bool {
	value, _ := input[name].(bool)
	return value
}

func intArg(input map[string]any, name string, fallback int) int {
	switch value := input[name].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		parsed, _ := strconv.Atoi(string(value))
		if parsed != 0 {
			return parsed
		}
	}
	return fallback
}

func boundedLine(line string) string {
	runes := []rune(strings.TrimSpace(line))
	if len(runes) > 300 {
		return string(runes[:300]) + "…"
	}
	return string(runes)
}

func objectSchema(properties map[string]*tools.JsonSchemaNode, required ...string) *tools.JsonSchemaNode {
	allow := false
	return &tools.JsonSchemaNode{
		Type: tools.TypeObject, Properties: properties, Required: required, AdditionalProperties: &allow,
	}
}

func stringSchema(description string) *tools.JsonSchemaNode {
	return &tools.JsonSchemaNode{Type: tools.TypeString, Description: description}
}

func integerSchema(description string) *tools.JsonSchemaNode {
	return &tools.JsonSchemaNode{Type: tools.TypeInteger, Description: description}
}
