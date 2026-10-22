package observability

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
)

const (
	maxSummaryLineBytes = 64 * 1024
	maxSummaryEvents    = 100_000
)

// CommandSummary is the bounded operational view of one CLI command.
type CommandSummary struct {
	Count     int            `json:"count"`
	Errors    int            `json:"errors"`
	ErrorRate float64        `json:"error_rate"`
	LatencyMS LatencySummary `json:"latency_ms"`
}

// LatencySummary contains interpolated percentile values in milliseconds.
type LatencySummary struct {
	P50 float64 `json:"p50"`
	P95 float64 `json:"p95"`
	P99 float64 `json:"p99"`
	Max float64 `json:"max"`
}

// Summary is the machine-readable input contract for an operational dashboard.
type Summary struct {
	Event        string                    `json:"event"`
	TotalEvents  int                       `json:"total_events"`
	IgnoredLines int                       `json:"ignored_lines"`
	Commands     map[string]CommandSummary `json:"commands"`
}

// Summarize reads JSONL command events from a CLI stderr stream. Non-JSON
// diagnostics are ignored because the CLI keeps human-readable errors on the
// same stderr stream as the opt-in event records.
func Summarize(input io.Reader) (Summary, error) {
	if input == nil {
		return Summary{}, fmt.Errorf("observability input is nil")
	}

	durations := make(map[string][]float64)
	errorsByCommand := make(map[string]int)
	counts := make(map[string]int)
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), maxSummaryLineBytes)
	summary := Summary{Event: "chitchat_cli_observability_summary", Commands: map[string]CommandSummary{}}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event commandEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil || event.Event != "chitchat_cli_command" || strings.TrimSpace(event.Command) == "" {
			summary.IgnoredLines++
			continue
		}
		if summary.TotalEvents >= maxSummaryEvents {
			return Summary{}, fmt.Errorf("observability input exceeds %d events", maxSummaryEvents)
		}
		if event.DurationMS < 0 || math.IsNaN(event.DurationMS) || math.IsInf(event.DurationMS, 0) {
			return Summary{}, fmt.Errorf("invalid duration for command %q", event.Command)
		}
		summary.TotalEvents++
		counts[event.Command]++
		durations[event.Command] = append(durations[event.Command], event.DurationMS)
		if !event.Success {
			errorsByCommand[event.Command]++
		}
	}
	if err := scanner.Err(); err != nil {
		return Summary{}, fmt.Errorf("read observability input: %w", err)
	}

	for command, values := range durations {
		sort.Float64s(values)
		count := counts[command]
		summary.Commands[command] = CommandSummary{
			Count: count, Errors: errorsByCommand[command],
			ErrorRate: float64(errorsByCommand[command]) / float64(count),
			LatencyMS: LatencySummary{
				P50: percentile(values, 0.50), P95: percentile(values, 0.95),
				P99: percentile(values, 0.99), Max: values[len(values)-1],
			},
		}
	}
	return summary, nil
}

func percentile(values []float64, quantile float64) float64 {
	if len(values) == 0 {
		return 0
	}
	position := quantile * float64(len(values)-1)
	low := int(math.Floor(position))
	high := int(math.Ceil(position))
	if low == high {
		return values[low]
	}
	weight := position - float64(low)
	return values[low] + (values[high]-values[low])*weight
}
