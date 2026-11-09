//

//

package tools

// ============================================================================

// ============================================================================

type ToolCallKind string

const (
	CallKindRead    ToolCallKind = "read"
	CallKindEdit    ToolCallKind = "edit"
	CallKindDelete  ToolCallKind = "delete"
	CallKindMove    ToolCallKind = "move"
	CallKindSearch  ToolCallKind = "search"
	CallKindExecute ToolCallKind = "execute"
	CallKindFetch   ToolCallKind = "fetch"
	CallKindOther   ToolCallKind = "other"
)

// ============================================================================

// ============================================================================

type FileLocation struct {
	Path string `json:"path"`

	Line int `json:"line,omitempty"`
}

type FileDiff struct {
	Path string `json:"path"`

	OldText *string `json:"oldText,omitempty"`

	NewText string `json:"newText"`
}

// ============================================================================

// ============================================================================

type ToolCallView interface {
	isCallView()
}

type GenericCallView struct {
	Card      string         `json:"card"`
	Title     string         `json:"title"`
	Kind      ToolCallKind   `json:"kind,omitempty"`
	RawInput  any            `json:"rawInput,omitempty"`
	Content   []any          `json:"content,omitempty"`
	Locations []FileLocation `json:"locations,omitempty"`
}

func (GenericCallView) isCallView() {}

func NewGenericCallView(title string) *GenericCallView {
	return &GenericCallView{Card: "generic", Title: title, Kind: CallKindOther}
}

type TerminalCallView struct {
	Card        string `json:"card"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Cwd         string `json:"cwd,omitempty"`
}

func (TerminalCallView) isCallView() {}

func NewTerminalCallView(command string) *TerminalCallView {
	return &TerminalCallView{Card: "terminal", Title: command}
}

type DiffCallView struct {
	Card      string         `json:"card"`
	Title     string         `json:"title"`
	Diffs     []FileDiff     `json:"diffs"`
	Locations []FileLocation `json:"locations,omitempty"`
}

func (DiffCallView) isCallView() {}

func NewDiffCallView(title string, diffs []FileDiff) *DiffCallView {
	return &DiffCallView{Card: "diff", Title: title, Diffs: diffs}
}

// ============================================================================

// ============================================================================

type ToolResultView interface {
	isResultView()
}

type GenericResultView struct {
	Card    string `json:"card"`
	Title   string `json:"title,omitempty"`
	Content []any  `json:"content,omitempty"`
}

func (GenericResultView) isResultView() {}

func NewGenericResultView() *GenericResultView {
	return &GenericResultView{Card: "generic"}
}

type TerminalResultView struct {
	Card     string `json:"card"`
	Title    string `json:"title,omitempty"`
	Output   string `json:"output,omitempty"`
	ExitCode *int   `json:"exitCode,omitempty"`
	Signal   string `json:"signal,omitempty"`
}

func (TerminalResultView) isResultView() {}

func NewTerminalResultView(output string) *TerminalResultView {
	return &TerminalResultView{Card: "terminal", Output: output}
}

type DiffResultView struct {
	Card  string     `json:"card"`
	Title string     `json:"title,omitempty"`
	Diffs []FileDiff `json:"diffs"`
}

func (DiffResultView) isResultView() {}

type SearchLineMatch struct {
	LineNumber int    `json:"lineNumber"`
	Line       string `json:"line"`
}

type SearchFileMatches struct {
	Path    string            `json:"path"`
	Matches []SearchLineMatch `json:"matches"`
}

type SearchResultView struct {
	Card      string              `json:"card"`
	Shape     string              `json:"shape"` // "matches" | "paths"
	Title     string              `json:"title,omitempty"`
	Files     []SearchFileMatches `json:"files,omitempty"`
	Paths     []string            `json:"paths,omitempty"`
	Truncated bool                `json:"truncated"`
	Total     int                 `json:"total"`
}

func (SearchResultView) isResultView() {}

func NewSearchMatchesView(files []SearchFileMatches, truncated bool, total int) *SearchResultView {
	return &SearchResultView{Card: "search", Shape: "matches", Files: files, Truncated: truncated, Total: total}
}

func NewSearchPathsView(paths []string, truncated bool, total int) *SearchResultView {
	return &SearchResultView{Card: "search", Shape: "paths", Paths: paths, Truncated: truncated, Total: total}
}

type ReadFileLine struct {
	Number int    `json:"number"`
	Text   string `json:"text"`
}

type ReadResultView struct {
	Card       string         `json:"card"`
	Title      string         `json:"title,omitempty"`
	Path       string         `json:"path"`
	Offset     int            `json:"offset"`
	Lines      []ReadFileLine `json:"lines"`
	TotalLines int            `json:"totalLines"`
	Lang       string         `json:"lang,omitempty"`
	Content    []any          `json:"content,omitempty"`
}

func (ReadResultView) isResultView() {}

func NewReadResultView(path string, offset int, lines []ReadFileLine, totalLines int) *ReadResultView {
	return &ReadResultView{Card: "read", Path: path, Offset: offset, Lines: lines, TotalLines: totalLines}
}

type WebSource struct {
	URL         string `json:"url"`
	Title       string `json:"title,omitempty"`
	Snippet     string `json:"snippet,omitempty"`
	PublishedAt string `json:"publishedAt,omitempty"`
}

type WebResultView struct {
	Card       string      `json:"card"`
	Kind       string      `json:"kind"` // "search" | "fetch"
	Title      string      `json:"title,omitempty"`
	Sources    []WebSource `json:"sources,omitempty"`
	Answer     string      `json:"answer,omitempty"`
	URL        string      `json:"url,omitempty"`
	StatusCode int         `json:"statusCode,omitempty"`
	Truncated  bool        `json:"truncated"`
}

func (WebResultView) isResultView() {}

func NewWebSearchResultView(sources []WebSource, answer string, truncated bool) *WebResultView {
	return &WebResultView{Card: "web", Kind: "search", Sources: sources, Answer: answer, Truncated: truncated}
}

func NewWebFetchResultView(url string, statusCode int, truncated bool) *WebResultView {
	return &WebResultView{Card: "web", Kind: "fetch", URL: url, StatusCode: statusCode, Truncated: truncated}
}

// ============================================================================

// ============================================================================

func CardOf(v any) string {
	switch tv := v.(type) {
	case ToolCallView:

		switch tv.(type) {
		case *GenericCallView:
			return "generic"
		case *TerminalCallView:
			return "terminal"
		case *DiffCallView:
			return "diff"
		case GenericCallView:
			return "generic"
		case TerminalCallView:
			return "terminal"
		case DiffCallView:
			return "diff"
		}
	case ToolResultView:
		switch r := tv.(type) {
		case *GenericResultView:
			return "generic"
		case *TerminalResultView:
			return "terminal"
		case *DiffResultView:
			return "diff"
		case SearchResultView:
			return "search"
		case *SearchResultView:
			return "search"
		case ReadResultView:
			return "read"
		case *ReadResultView:
			return "read"
		case WebResultView:
			return "web"
		case *WebResultView:
			return "web"
		case GenericResultView:
			return "generic"
		case TerminalResultView:
			return "terminal"
		case DiffResultView:
			return "diff"
		default:
			_ = r
		}
	}
	return "other"
}
