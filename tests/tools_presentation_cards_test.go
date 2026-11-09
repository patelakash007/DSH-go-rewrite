//

package tests

import (
	"testing"

	"github.com/JopenChen/dsh-go/pkg/tools"
)

type customCard struct {
	Card string `json:"card"`
	Note string `json:"note"`
}

func TestToolPresentationAllCallCards(t *testing.T) {
	gen := tools.NewGenericCallView("Search Files")
	gen.Kind = tools.CallKindSearch
	cases := []struct {
		name string
		view tools.ToolCallView
		want string
	}{
		{"generic", gen, "generic"},
		{"terminal", tools.NewTerminalCallView("npm run build"), "terminal"},
		{"diff", tools.NewDiffCallView("Write foo.txt", []tools.FileDiff{{Path: "foo.txt", NewText: "hi"}}), "diff"},
	}
	for _, c := range cases {
		if got := tools.CardOf(c.view); got != c.want {
			t.Errorf("%s: CardOf = %s, want %s", c.name, got, c.want)
		}
	}
}

func TestToolPresentationAllResultCards(t *testing.T) {
	cases := []struct {
		name string
		view tools.ToolResultView
		want string
	}{
		{"generic", tools.NewGenericResultView(), "generic"},
		{"terminal", tools.NewTerminalResultView("output"), "terminal"},
		{"diff", &tools.DiffResultView{Card: "diff", Diffs: []tools.FileDiff{{Path: "a", NewText: "b"}}}, "diff"},
		{"search-matches", tools.NewSearchMatchesView(nil, false, 1), "search"},
		{"search-paths", tools.NewSearchPathsView([]string{"a.go"}, false, 1), "search"},
		{"read", tools.NewReadResultView("/x/a.go", 1, nil, 2), "read"},
		{"web-search", tools.NewWebSearchResultView(nil, "a", false), "web"},
		{"web-fetch", tools.NewWebFetchResultView("https://example.com", 200, false), "web"},
	}
	for _, c := range cases {
		if got := tools.CardOf(c.view); got != c.want {
			t.Errorf("%s: CardOf = %s, want %s", c.name, got, c.want)
		}
	}
}

func TestToolPresentationSearchShapes(t *testing.T) {
	m := tools.NewSearchMatchesView([]tools.SearchFileMatches{
		{Path: "a.go", Matches: []tools.SearchLineMatch{{LineNumber: 1, Line: "func main()"}}},
	}, true, 100)
	if m.Card != "search" || m.Shape != "matches" || !m.Truncated || m.Total != 100 {
		t.Fatalf("search-matches field error: %+v", m)
	}
	p := tools.NewSearchPathsView([]string{"a.go", "b.go"}, false, 2)
	if p.Shape != "paths" || len(p.Paths) != 2 || p.Truncated {
		t.Fatalf("search-paths field error: %+v", p)
	}
}

func TestToolPresentationWebKinds(t *testing.T) {
	s := tools.NewWebSearchResultView([]tools.WebSource{{URL: "https://x", Title: "X"}}, "answer", false)
	if s.Kind != "search" || len(s.Sources) != 1 || s.Answer != "answer" {
		t.Fatalf("web-search field error: %+v", s)
	}
	f := tools.NewWebFetchResultView("https://y", 200, false)
	if f.Kind != "fetch" || f.URL != "https://y" || f.StatusCode != 200 {
		t.Fatalf("web-fetch field error: %+v", f)
	}
}

func TestToolPresentationCustomCard(t *testing.T) {
	var view any = customCard{Card: "mycard", Note: "custom"}
	if got := tools.CardOf(view); got != "other" {
		t.Fatalf("custom card CardOf should fallback other, got %s", got)
	}
}
