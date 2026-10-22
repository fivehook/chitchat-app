package observability

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// AlertCheck is one explicit operational threshold evaluation.
type AlertCheck struct {
	Name      string  `json:"name"`
	Scope     string  `json:"scope"`
	Actual    float64 `json:"actual"`
	Threshold float64 `json:"threshold"`
	Triggered bool    `json:"triggered"`
}

// AlertReport is suitable for a cron job or external alert sink.
type AlertReport struct {
	Event   string       `json:"event"`
	Healthy bool         `json:"healthy"`
	Checks  []AlertCheck `json:"checks"`
}

// Evaluate applies only thresholds explicitly supplied by the operator. A
// disabled threshold is represented by maxP95MS <= 0, maxErrorRate < 0, or
// maxIgnoredLines < 0.
func Evaluate(summary Summary, maxP95MS, maxErrorRate float64, maxIgnoredLines int) (AlertReport, error) {
	if maxP95MS <= 0 && maxErrorRate < 0 && maxIgnoredLines < 0 {
		return AlertReport{}, fmt.Errorf("at least one alert threshold must be configured")
	}
	if maxP95MS < 0 || maxErrorRate < -1 || maxErrorRate > 1 || maxIgnoredLines < -1 {
		return AlertReport{}, fmt.Errorf("invalid alert threshold")
	}

	commands := make([]string, 0, len(summary.Commands))
	for command := range summary.Commands {
		commands = append(commands, command)
	}
	sort.Strings(commands)
	checks := make([]AlertCheck, 0, len(commands)*2+1)
	for _, command := range commands {
		value := summary.Commands[command]
		if maxP95MS > 0 {
			checks = append(checks, AlertCheck{Name: "p95_latency_ms", Scope: command, Actual: value.LatencyMS.P95, Threshold: maxP95MS, Triggered: value.LatencyMS.P95 > maxP95MS})
		}
		if maxErrorRate >= 0 {
			checks = append(checks, AlertCheck{Name: "error_rate", Scope: command, Actual: value.ErrorRate, Threshold: maxErrorRate, Triggered: value.ErrorRate > maxErrorRate})
		}
	}
	if maxIgnoredLines >= 0 {
		checks = append(checks, AlertCheck{Name: "ignored_lines", Scope: "all", Actual: float64(summary.IgnoredLines), Threshold: float64(maxIgnoredLines), Triggered: summary.IgnoredLines > maxIgnoredLines})
	}
	healthy := true
	for _, check := range checks {
		if check.Triggered {
			healthy = false
			break
		}
	}
	return AlertReport{Event: "chitchat_cli_observability_alerts", Healthy: healthy, Checks: checks}, nil
}

// RetentionReport checks one event file without deleting or modifying it.
type RetentionReport struct {
	Event      string  `json:"event"`
	Path       string  `json:"path"`
	Exists     bool    `json:"exists"`
	AgeSeconds float64 `json:"age_seconds"`
	MaxAge     string  `json:"max_age"`
	Expired    bool    `json:"expired"`
}

// CheckRetention reports whether a log file exceeds maxAge. Cleanup remains
// an explicit deployment action so a check cannot accidentally delete data.
func CheckRetention(path string, maxAge time.Duration, now time.Time) (RetentionReport, error) {
	if path == "" {
		return RetentionReport{}, fmt.Errorf("retention path is required")
	}
	if maxAge <= 0 {
		return RetentionReport{}, fmt.Errorf("retention max age must be positive")
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return RetentionReport{Event: "chitchat_cli_observability_retention", Path: path, MaxAge: maxAge.String()}, nil
		}
		return RetentionReport{}, fmt.Errorf("stat retention path: %w", err)
	}
	age := now.Sub(info.ModTime())
	return RetentionReport{Event: "chitchat_cli_observability_retention", Path: path, Exists: true, AgeSeconds: age.Seconds(), MaxAge: maxAge.String(), Expired: age > maxAge}, nil
}

// RotationReport describes an archive-only retention rotation. The source is
// never deleted; apply=true moves it into archiveDir with a UTC timestamp.
type RotationReport struct {
	Event       string `json:"event"`
	Path        string `json:"path"`
	ArchivePath string `json:"archive_path,omitempty"`
	Exists      bool   `json:"exists"`
	Expired     bool   `json:"expired"`
	Applied     bool   `json:"applied"`
}

// RotateRetention performs a reversible archive move only when apply is true.
func RotateRetention(path, archiveDir string, maxAge time.Duration, now time.Time, apply bool) (RotationReport, error) {
	if path == "" {
		return RotationReport{}, fmt.Errorf("retention path is required")
	}
	if archiveDir == "" {
		return RotationReport{}, fmt.Errorf("retention archive directory is required")
	}
	if maxAge <= 0 {
		return RotationReport{}, fmt.Errorf("retention max age must be positive")
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return RotationReport{Event: "chitchat_cli_observability_rotation", Path: path}, nil
		}
		return RotationReport{}, fmt.Errorf("stat retention path: %w", err)
	}
	if info.IsDir() {
		return RotationReport{}, fmt.Errorf("retention path must be a file")
	}
	report := RotationReport{Event: "chitchat_cli_observability_rotation", Path: path, Exists: true}
	if now.Sub(info.ModTime()) <= maxAge {
		return report, nil
	}
	report.Expired = true
	archivePath := filepath.Join(archiveDir, filepath.Base(path)+"."+now.UTC().Format("20060102T150405Z"))
	if _, err := os.Stat(archivePath); err == nil {
		return RotationReport{}, fmt.Errorf("retention archive already exists: %s", archivePath)
	} else if !os.IsNotExist(err) {
		return RotationReport{}, fmt.Errorf("check retention archive: %w", err)
	}
	report.ArchivePath = archivePath
	if !apply {
		return report, nil
	}
	if err := os.MkdirAll(archiveDir, 0o700); err != nil {
		return RotationReport{}, fmt.Errorf("create retention archive directory: %w", err)
	}
	if err := os.Rename(path, archivePath); err != nil {
		return RotationReport{}, fmt.Errorf("archive retention file: %w", err)
	}
	report.Applied = true
	return report, nil
}
