//

package tests

import (
	"strings"
	"testing"

	"github.com/JopenChen/dsh-go/pkg/brand"
	"github.com/JopenChen/dsh-go/pkg/spill"
)

func newSpillReq(sessionID, tool, suggested string) spill.SaveTextSpill {
	return spill.SaveTextSpill{
		Owner:         spill.SpillOwner{SessionID: brand.NewSessionID(sessionID)},
		Source:        spill.SpillSource{ToolName: tool, CallID: brand.NewToolCallID("call-1"), Label: "result"},
		SuggestedName: suggested,
	}
}

func TestSpillToolResultOverThreshold(t *testing.T) {
	store := spill.NewFileStore(t.TempDir())
	bigText := strings.Repeat("line of tool output\n", 500)

	inline, ref, err := spill.Apply(store, newSpillReq("s1", "web_fetch", "web_fetch.txt"), bigText, 1024, 256)
	if err != nil {
		t.Fatalf("spill failed: %v", err)
	}
	if ref == nil {
		t.Fatal("exceeding threshold should generate spill ref")
	}
	if len(inline) >= len(bigText) {
		t.Fatal("inline should be preview rather than full text")
	}

	got, err := store.ReadText(ref.Locator)
	if err != nil {
		t.Fatalf("read locator failed: %v", err)
	}
	if got != bigText {
		t.Fatalf("restored content mismatch: got %d bytes, want %d bytes", len(got), len(bigText))
	}
}

func TestSpillBashResultOverThreshold(t *testing.T) {
	store := spill.NewFileStore(t.TempDir())
	seqOutput := strings.Repeat("1\n2\n3\n4\n5\n", 1000) // ~10KB

	_, ref, err := spill.Apply(store, newSpillReq("s2", "bash", "bash.txt"), seqOutput, 64, 64)
	if err != nil {
		t.Fatalf("bash spill failed: %v", err)
	}
	if ref == nil {
		t.Fatal("bash result exceeding threshold should spill")
	}
	if ref.Bytes != len(seqOutput) {
		t.Fatalf("ref.Bytes should equal original length %d, got %d", len(seqOutput), ref.Bytes)
	}
	got, _ := store.ReadText(ref.Locator)
	if got != seqOutput {
		t.Fatal("Bash result read-back should be byte-for-byte identical")
	}
}

func TestSpillUnderThresholdInline(t *testing.T) {
	store := spill.NewFileStore(t.TempDir())
	small := "short"
	inline, ref, err := spill.Apply(store, newSpillReq("s3", "tool", "a.txt"), small, 1024, 0)
	if err != nil {
		t.Fatal(err)
	}
	if ref != nil {
		t.Fatal("should not spill when below threshold")
	}
	if inline != small {
		t.Fatalf("should retain original inline, got %q", inline)
	}
}

func TestSpillBestEffortOnSaveFailure(t *testing.T) {

	store := spill.NewFileStore("")
	big := strings.Repeat("x", 4096)
	_, _, err := spill.Apply(store, newSpillReq("s4", "bash", "b.txt"), big, 64, 0)
	if err == nil {
		t.Fatal("unconfigured root should fail to save and return error")
	}

}
