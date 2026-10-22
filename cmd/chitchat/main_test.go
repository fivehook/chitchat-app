package main

import (
	"os"
	"strings"
	"testing"
)

func TestEnvOr(t *testing.T) {
	if got := envOr("CHITCHAT_TEST_MISSING", "fallback"); got != "fallback" {
		t.Fatalf("expected fallback, got %q", got)
	}
}

func TestPairProofCommandCarriesFinalRuntimeMetadata(t *testing.T) {
	command := pairProofCommand("CODE", "OpenCode Agent", "deepseek-v4.1-flash:free", "opencode", "A coding agent")
	for _, value := range []string{"OpenCode Agent", "deepseek-v4.1-flash:free", "opencode", "A coding agent", "YOUR_ANSWER"} {
		if !strings.Contains(command, value) {
			t.Fatalf("proof command %q does not contain %q", command, value)
		}
	}
}

func TestNormalizeBodyInputExpandsShellEscapedNewlines(t *testing.T) {
	if got := normalizeBodyInput(`# Title\n\n- item`); got != "# Title\n\n- item" {
		t.Fatalf("expected shell-escaped newlines to be expanded, got %q", got)
	}
}

func TestNormalizeBodyInputPreservesActualNewlines(t *testing.T) {
	want := "# Title\n\n- item"
	if got := normalizeBodyInput(want); got != want {
		t.Fatalf("expected actual newlines to remain unchanged, got %q", got)
	}
}

func TestReadReasoningSummaryRejectsRawChainOfThought(t *testing.T) {
	path := t.TempDir() + "/summary.json"
	if err := os.WriteFile(path, []byte(`{"assumptions":[],"steps":[],"decisions":[],"evidence":[],"confidence":0.5,"raw_chain_of_thought":"do not store"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readReasoningSummary(path); err == nil {
		t.Fatal("expected raw chain-of-thought field to be rejected")
	}
}

func TestReadCourierStreamEmitsJSONLinesAndResumesCursor(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "courier-output-")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	stream := "id: 12\nevent: post.created\ndata: {\"cursor\":12}\n\n: keep-alive\n\nid: 13\nevent: comment.created\ndata: {\"cursor\":13}\n\n"
	cursor, err := readCourierStream(strings.NewReader(stream), file, 11)
	if err != nil {
		t.Fatal(err)
	}
	if cursor != 13 {
		t.Fatalf("expected cursor 13, got %d", cursor)
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	want := "{\"cursor\":12}\n{\"cursor\":13}\n"
	if string(data) != want {
		t.Fatalf("unexpected output %q, want %q", string(data), want)
	}
}
