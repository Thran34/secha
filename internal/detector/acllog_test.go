package detector

import (
	"strings"
	"testing"
	"time"
)

func TestWatchScriptPollsForTheWholeWindowAndThenGivesUp(t *testing.T) {
	script := watchScript(10 * time.Second)

	if !strings.Contains(script, "-lt 200 ") {
		t.Errorf("script = %q, want 200 polls for a 10s window at 50ms", script)
	}
	if !strings.Contains(script, "sleep 0.050") {
		t.Errorf("script = %q, want a 50ms sleep between polls", script)
	}
	if !strings.Contains(script, "echo NONE") {
		t.Error("script must report NONE when the window closes, silence would be ambiguous")
	}
}

func TestWatchScriptStopsAtTheFirstDenial(t *testing.T) {
	script := watchScript(time.Second)

	if !strings.Contains(script, "echo DETECTED; exit 0") {
		t.Error("script must exit on the first auth denial so the exec returns at detection time")
	}
}

func TestWatchScriptNeverRunsZeroPolls(t *testing.T) {
	if !strings.Contains(watchScript(time.Millisecond), "-lt 1 ") {
		t.Error("a window shorter than one poll interval must still poll once")
	}
}

func TestIsAuthDenialMatchesTheACLLogEntry(t *testing.T) {
	entry := `1) 1) "count"
   2) (integer) 1
   3) "reason"
   4) "auth"
   5) "context"
   6) "toplevel"`
	if !isAuthDenial(entry) {
		t.Error("isAuthDenial = false for a real ACL LOG auth entry")
	}
	if isAuthDenial("(empty array)") {
		t.Error("isAuthDenial = true for an empty log")
	}
}
