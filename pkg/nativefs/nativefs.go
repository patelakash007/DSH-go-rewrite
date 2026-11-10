// Package nativefs provides the app-owned filesystem boundary used by the
// Android-native server and model tools. It deliberately does not expose the
// rest of the app sandbox: every path is resolved beneath one canonical
// workspace root, including through symlinks.
package nativefs

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	DefaultMaxEntries    = 2000
	DefaultMaxTextBytes  = 2 << 20
	DefaultMaxWindow     = 2 << 20
	DefaultMaxWholeBytes = 20 << 20
)

type ErrorCode string

const (
	ErrInvalid    ErrorCode = "workspace-file/invalid"
	ErrNotFound   ErrorCode = "workspace-file/not-found"
	ErrNotAllowed ErrorCode = "workspace-file/not-allowed"
	ErrNotFile    ErrorCode = "workspace-file/not-file"
	ErrNotDir     ErrorCode = "workspace-file/not-directory"
	ErrNotText    ErrorCode = "workspace-file/not-text"
	ErrTooLarge   ErrorCode = "workspace-file/too-large"
	ErrIO         ErrorCode = "workspace-file/io"
)

type Error struct {
	Code ErrorCode
	Path string
	Err  error
}

func (e *Error) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("%s: %s", e.Code, e.Path)
	}
	return fmt.Sprintf("%s: %s: %v", e.Code, e.Path, e.Err)
}

func (e *Error) Unwrap() error { return e.Err }

func Code(err error) ErrorCode {
	var target *Error
	if errors.As(err, &target) {
		return target.Code
	}
	return ErrIO
}

type Service struct {
	root string
}

func New(root string) (*Service, error) {
	if strings.TrimSpace(root) == "" {
		return nil, &Error{Code: ErrInvalid, Path: root, Err: errors.New("workspace root is empty")}
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, &Error{Code: ErrInvalid, Path: root, Err: err}
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, &Error{Code: ErrIO, Path: abs, Err: err}
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, &Error{Code: ErrIO, Path: abs, Err: err}
	}
	return &Service{root: filepath.Clean(real)}, nil
}

func (s *Service) Root() string { return s.root }

// Resolve maps historical /root/workspace paths to the native workspace and
// confines both existing targets and not-yet-existing write targets.
func (s *Service) Resolve(cwd, path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "."
	}
	path = mapLegacyWorkspace(path)
	cwd = mapLegacyWorkspace(cwd)
	if cwd == "" || cwd == "." {
		cwd = s.root
	}
	if !filepath.IsAbs(cwd) {
		cwd = filepath.Join(s.root, cwd)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", &Error{Code: ErrInvalid, Path: path, Err: err}
	}
	abs = filepath.Clean(abs)
	real, err := resolveExistingPrefix(abs)
	if err != nil {
		return "", &Error{Code: classifyOS(err), Path: path, Err: err}
	}
	if !within(s.root, real) {
		return "", &Error{Code: ErrNotAllowed, Path: path, Err: errors.New("path leaves the native workspace")}
	}
	return real, nil
}

func mapLegacyWorkspace(path string) string {
	clean := filepath.ToSlash(filepath.Clean(path))
	const legacy = "/root/workspace"
	if clean == legacy {
		return "."
	}
	if strings.HasPrefix(clean, legacy+"/") {
		return strings.TrimPrefix(clean, legacy+"/")
	}
	return path
}

func resolveExistingPrefix(path string) (string, error) {
	current := path
	var suffix []string
	for {
		real, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				real = filepath.Join(real, suffix[i])
			}
			return filepath.Clean(real), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}

func within(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

type Entry struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Size int64  `json:"size,omitempty"`
}

type Listing struct {
	Path      string  `json:"path"`
	Entries   []Entry `json:"entries"`
	Truncated bool    `json:"truncated"`
}

func (s *Service) List(cwd, path string, maxEntries int) (Listing, error) {
	resolved, err := s.Resolve(cwd, path)
	if err != nil {
		return Listing{}, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return Listing{}, pathError(path, err)
	}
	if !info.IsDir() {
		return Listing{}, &Error{Code: ErrNotDir, Path: path}
	}
	children, err := os.ReadDir(resolved)
	if err != nil {
		return Listing{}, pathError(path, err)
	}
	sort.Slice(children, func(i, j int) bool { return children[i].Name() < children[j].Name() })
	if maxEntries <= 0 {
		maxEntries = DefaultMaxEntries
	}
	truncated := len(children) > maxEntries
	if truncated {
		children = children[:maxEntries]
	}
	entries := make([]Entry, 0, len(children))
	for _, child := range children {
		entry := Entry{Name: child.Name(), Type: "other"}
		childInfo, statErr := os.Stat(filepath.Join(resolved, child.Name()))
		if statErr == nil {
			switch {
			case childInfo.IsDir():
				entry.Type = "directory"
			case childInfo.Mode().IsRegular():
				entry.Type = "file"
				entry.Size = childInfo.Size()
			}
		}
		entries = append(entries, entry)
	}
	return Listing{Path: relativeDisplay(s.root, resolved), Entries: entries, Truncated: truncated}, nil
}

type Stat struct {
	AbsolutePath string `json:"absolutePath"`
	Version      string `json:"version"`
	Bytes        int64  `json:"bytes"`
}

func (s *Service) Stat(cwd, path string) (Stat, error) {
	resolved, err := s.Resolve(cwd, path)
	if err != nil {
		return Stat{}, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return Stat{}, pathError(path, err)
	}
	if !info.Mode().IsRegular() {
		return Stat{}, &Error{Code: ErrNotFile, Path: path}
	}
	version, err := fileVersion(resolved, info)
	if err != nil {
		return Stat{}, pathError(path, err)
	}
	return Stat{AbsolutePath: resolved, Version: version, Bytes: info.Size()}, nil
}

type TextPage struct {
	Text         string `json:"text"`
	Lines        int    `json:"lines"`
	EOF          bool   `json:"eof"`
	Offset       int    `json:"offset"`
	AbsolutePath string `json:"absolutePath"`
	Version      string `json:"version"`
	Bytes        int64  `json:"bytes"`
}

func (s *Service) ReadText(cwd, path string, offset, limit, maxBytes int) (TextPage, error) {
	stat, err := s.Stat(cwd, path)
	if err != nil {
		return TextPage{}, err
	}
	if offset < 1 || limit < 1 {
		return TextPage{}, &Error{Code: ErrInvalid, Path: path, Err: errors.New("line range must be positive")}
	}
	if limit > 5000 {
		limit = 5000
	}
	if maxBytes <= 0 {
		maxBytes = DefaultMaxTextBytes
	}
	file, err := os.Open(stat.AbsolutePath)
	if err != nil {
		return TextPage{}, pathError(path, err)
	}
	defer file.Close()
	reader := bufio.NewReaderSize(file, 64<<10)
	selected := make([]string, 0, limit)
	lineNo := 0
	pageBytes := 0
	eof := false
	for {
		line, readErr := reader.ReadString('\n')
		if len(line) > 0 {
			lineNo++
			if lineNo >= offset && len(selected) < limit {
				line = strings.TrimSuffix(line, "\n")
				line = strings.TrimSuffix(line, "\r")
				pageBytes += len(line)
				if len(selected) > 0 {
					pageBytes++
				}
				if pageBytes > maxBytes {
					return TextPage{}, &Error{Code: ErrTooLarge, Path: path, Err: fmt.Errorf("text page exceeds %d bytes", maxBytes)}
				}
				if strings.IndexByte(line, 0) >= 0 || !utf8.ValidString(line) {
					return TextPage{}, &Error{Code: ErrNotText, Path: path}
				}
				selected = append(selected, line)
			}
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				return TextPage{}, pathError(path, readErr)
			}
			eof = true
			break
		}
		if len(selected) == limit {
			_, peekErr := reader.Peek(1)
			eof = errors.Is(peekErr, io.EOF)
			break
		}
	}
	return TextPage{
		Text: strings.Join(selected, "\n"), Lines: len(selected), EOF: eof, Offset: offset,
		AbsolutePath: stat.AbsolutePath, Version: stat.Version, Bytes: stat.Bytes,
	}, nil
}

type Window struct {
	Data         []byte `json:"data"`
	EOF          bool   `json:"eof"`
	Offset       int64  `json:"offset"`
	AbsolutePath string `json:"absolutePath"`
	Version      string `json:"version"`
	Bytes        int64  `json:"bytes"`
}

func (s *Service) ReadWindow(cwd, path string, offset int64, length int) (Window, error) {
	stat, err := s.Stat(cwd, path)
	if err != nil {
		return Window{}, err
	}
	if offset < 0 || length < 0 || length > DefaultMaxWindow {
		return Window{}, &Error{Code: ErrInvalid, Path: path, Err: fmt.Errorf("invalid byte range offset=%d length=%d", offset, length)}
	}
	file, err := os.Open(stat.AbsolutePath)
	if err != nil {
		return Window{}, pathError(path, err)
	}
	defer file.Close()
	if offset > stat.Bytes {
		offset = stat.Bytes
	}
	data := make([]byte, length)
	n, readErr := file.ReadAt(data, offset)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return Window{}, pathError(path, readErr)
	}
	data = data[:n]
	return Window{
		Data: data, EOF: offset+int64(n) >= stat.Bytes, Offset: offset,
		AbsolutePath: stat.AbsolutePath, Version: stat.Version, Bytes: stat.Bytes,
	}, nil
}

func (s *Service) ReadWhole(cwd, path string, maxBytes int64) ([]byte, Stat, error) {
	stat, err := s.Stat(cwd, path)
	if err != nil {
		return nil, Stat{}, err
	}
	if maxBytes <= 0 {
		maxBytes = DefaultMaxWholeBytes
	}
	if stat.Bytes > maxBytes {
		return nil, Stat{}, &Error{Code: ErrTooLarge, Path: path, Err: fmt.Errorf("file is %d bytes (limit %d)", stat.Bytes, maxBytes)}
	}
	data, err := os.ReadFile(stat.AbsolutePath)
	if err != nil {
		return nil, Stat{}, pathError(path, err)
	}
	return data, stat, nil
}

func (s *Service) WriteText(cwd, path, content string) (Stat, error) {
	resolved, err := s.Resolve(cwd, path)
	if err != nil {
		return Stat{}, err
	}
	if !utf8.ValidString(content) {
		return Stat{}, &Error{Code: ErrNotText, Path: path}
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o700); err != nil {
		return Stat{}, pathError(path, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(resolved), ".dsh-write-*")
	if err != nil {
		return Stat{}, pathError(path, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return Stat{}, pathError(path, err)
	}
	if _, err := io.WriteString(tmp, content); err != nil {
		_ = tmp.Close()
		return Stat{}, pathError(path, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return Stat{}, pathError(path, err)
	}
	if err := tmp.Close(); err != nil {
		return Stat{}, pathError(path, err)
	}
	if err := os.Rename(tmpName, resolved); err != nil {
		return Stat{}, pathError(path, err)
	}
	return s.Stat(cwd, path)
}

func (s *Service) EditText(cwd, path, oldText, newText string, replaceAll bool) (Stat, error) {
	data, _, err := s.ReadWhole(cwd, path, DefaultMaxTextBytes)
	if err != nil {
		return Stat{}, err
	}
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return Stat{}, &Error{Code: ErrNotText, Path: path}
	}
	content := string(data)
	count := strings.Count(content, oldText)
	if oldText == "" || count == 0 {
		return Stat{}, &Error{Code: ErrInvalid, Path: path, Err: errors.New("edit text was not found")}
	}
	if count > 1 && !replaceAll {
		return Stat{}, &Error{Code: ErrInvalid, Path: path, Err: errors.New("edit is ambiguous; set replaceAll")}
	}
	n := 1
	if replaceAll {
		n = -1
	}
	return s.WriteText(cwd, path, strings.Replace(content, oldText, newText, n))
}

func (s *Service) Signature(cwd, path string) (string, bool, error) {
	resolved, err := s.Resolve(cwd, path)
	if err != nil {
		return "", false, err
	}
	info, err := os.Stat(resolved)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, pathError(path, err)
	}
	if info.IsDir() {
		entries, err := os.ReadDir(resolved)
		if err != nil {
			return "", false, pathError(path, err)
		}
		h := sha256.New()
		for _, entry := range entries {
			childInfo, _ := entry.Info()
			fmt.Fprintf(h, "%s\x00%d\x00%d\n", entry.Name(), childInfo.Size(), childInfo.ModTime().UnixNano())
		}
		return hex.EncodeToString(h.Sum(nil)[:12]), true, nil
	}
	version, err := fileVersion(resolved, info)
	return version, true, err
}

func fileVersion(path string, info os.FileInfo) (string, error) {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%d\x00%d\x00%d", path, info.Size(), info.ModTime().UnixNano(), info.Mode())
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	const sample = 64 << 10
	first := make([]byte, sample)
	n, err := file.Read(first)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	_, _ = h.Write(first[:n])
	if info.Size() > sample {
		last := make([]byte, sample)
		n, err = file.ReadAt(last, max64(0, info.Size()-sample))
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
		_, _ = h.Write(last[:n])
	}
	return hex.EncodeToString(h.Sum(nil)[:12]), nil
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func relativeDisplay(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return ""
	}
	return filepath.ToSlash(rel)
}

func pathError(path string, err error) error {
	return &Error{Code: classifyOS(err), Path: path, Err: err}
}

func classifyOS(err error) ErrorCode {
	switch {
	case os.IsNotExist(err):
		return ErrNotFound
	case os.IsPermission(err):
		return ErrNotAllowed
	default:
		return ErrIO
	}
}

// ModTime is exposed for deterministic watcher tests without leaking an
// os.FileInfo through the server contract.
func ModTime(path string) (time.Time, error) {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}
