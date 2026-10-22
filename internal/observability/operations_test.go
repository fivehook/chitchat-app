package observability

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEvaluateThresholds(t *testing.T) {
	summary := Summary{IgnoredLines: 2, Commands: map[string]CommandSummary{
		"get.list": {ErrorRate: 0.25, LatencyMS: LatencySummary{P95: 120}},
	}}
	report, err := Evaluate(summary, 100, 0.2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if report.Healthy || len(report.Checks) != 3 {
		t.Fatalf("unexpected alert report: %#v", report)
	}
	if !report.Checks[0].Triggered || !report.Checks[1].Triggered || !report.Checks[2].Triggered {
		t.Fatalf("expected all checks to trigger: %#v", report.Checks)
	}
}

func TestCheckRetentionIsNonDestructive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, []byte("event\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	report, err := CheckRetention(path, time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Exists || report.Expired {
		t.Fatalf("unexpected retention report: %#v", report)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("retention check removed input: %v", err)
	}
}

func TestRotateRetentionArchivesOnlyWhenApplied(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	if err := os.WriteFile(path, []byte("event\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(path, now.Add(-48*time.Hour), now.Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	archiveDir := filepath.Join(dir, "archive")

	dryRun, err := RotateRetention(path, archiveDir, time.Hour, now, false)
	if err != nil {
		t.Fatal(err)
	}
	if !dryRun.Expired || dryRun.Applied {
		t.Fatalf("unexpected dry-run report: %#v", dryRun)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("dry-run moved source: %v", err)
	}

	applied, err := RotateRetention(path, archiveDir, time.Hour, now, true)
	if err != nil {
		t.Fatal(err)
	}
	if !applied.Applied || applied.ArchivePath == "" {
		t.Fatalf("unexpected applied report: %#v", applied)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("source still exists or stat failed: %v", err)
	}
	if _, err := os.Stat(applied.ArchivePath); err != nil {
		t.Fatalf("archive missing: %v", err)
	}
}
