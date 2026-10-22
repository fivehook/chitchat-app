package observability

import (
	"strings"
	"testing"
)

func TestSummarizeCommandEvents(t *testing.T) {
	input := strings.NewReader("not-json\n" +
		`{"event":"other","command":"ignored","duration_ms":10,"success":true}` + "\n" +
		`{"event":"chitchat_cli_command","command":"get.list","duration_ms":10,"success":true}` + "\n" +
		`{"event":"chitchat_cli_command","command":"get.list","duration_ms":30,"success":false,"error_class":"command_error"}` + "\n" +
		`{"event":"chitchat_cli_command","command":"get.post","duration_ms":20,"success":true}` + "\n")

	summary, err := Summarize(input)
	if err != nil {
		t.Fatal(err)
	}
	if summary.TotalEvents != 3 || summary.IgnoredLines != 2 {
		t.Fatalf("unexpected counts: %#v", summary)
	}
	list := summary.Commands["get.list"]
	if list.Count != 2 || list.Errors != 1 || list.ErrorRate != 0.5 {
		t.Fatalf("unexpected list summary: %#v", list)
	}
	if list.LatencyMS.P50 != 20 || list.LatencyMS.P95 != 29 || list.LatencyMS.P99 != 29.8 || list.LatencyMS.Max != 30 {
		t.Fatalf("unexpected list latency: %#v", list.LatencyMS)
	}
}

func TestSummarizeRejectsInvalidDuration(t *testing.T) {
	input := strings.NewReader(`{"event":"chitchat_cli_command","command":"get.list","duration_ms":-1,"success":true}` + "\n")
	if _, err := Summarize(input); err == nil {
		t.Fatal("expected invalid duration error")
	}
}
