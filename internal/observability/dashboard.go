package observability

import (
	"html/template"
	"io"
	"sort"
	"time"
)

type dashboardRow struct {
	Command   string
	Count     int
	Errors    int
	ErrorRate float64
	Latency   LatencySummary
}

type dashboardData struct {
	GeneratedAt  string
	TotalEvents  int
	IgnoredLines int
	Rows         []dashboardRow
}

var dashboardTemplate = template.Must(template.New("observability-dashboard").Funcs(template.FuncMap{
	"mul100": func(value float64) float64 { return value * 100 },
}).Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Chitchat CLI observability</title>
  <style>
    :root { color-scheme: light dark; --bg: #101412; --surface: #18201c; --text: #edf3ee; --muted: #b7c5ba; --line: #405247; --accent: #c8ef62; --danger: #ffb4ab; }
    * { box-sizing: border-box; }
    body { margin: 0; padding: 32px 20px; background: var(--bg); color: var(--text); font: 16px/1.5 system-ui, sans-serif; }
    main { max-width: 1120px; margin: 0 auto; }
    h1 { margin: 0 0 8px; font-size: clamp(1.8rem, 4vw, 2.7rem); letter-spacing: -0.03em; }
    .lede { max-width: 720px; margin: 0 0 24px; color: var(--muted); }
    .summary { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; margin: 24px 0; }
    .stat, .table-shell { background: var(--surface); border: 1px solid var(--line); border-radius: 8px; }
    .stat { padding: 16px; }
    .stat-label { display: block; color: var(--muted); font-size: .84rem; }
    .stat-value { display: block; margin-top: 4px; font-size: 1.55rem; font-weight: 650; }
    .table-shell { overflow-x: auto; }
    table { width: 100%; min-width: 720px; border-collapse: collapse; }
    caption { padding: 16px; text-align: left; font-weight: 650; }
    th, td { padding: 12px 16px; border-top: 1px solid var(--line); text-align: right; white-space: nowrap; }
    th:first-child, td:first-child { text-align: left; }
    th { color: var(--muted); font-size: .82rem; font-weight: 600; }
    td:first-child { color: var(--accent); font-family: ui-monospace, monospace; }
    .error { color: var(--danger); }
    .empty { padding: 20px 16px; color: var(--muted); }
    footer { margin-top: 16px; color: var(--muted); font-size: .85rem; }
    @media (max-width: 640px) { body { padding: 24px 14px; } .summary { grid-template-columns: 1fr; } }
    @media (prefers-color-scheme: light) {
      :root { --bg: #f5f7f3; --surface: #ffffff; --text: #172019; --muted: #4d5d51; --line: #c7d2c8; --accent: #365d0d; --danger: #a3261c; }
    }
  </style>
</head>
<body>
  <main>
    <h1>Chitchat CLI observability</h1>
    <p class="lede">Local summary of bounded JSONL command events. This file is a report, not a live production dashboard.</p>
    <section class="summary" aria-label="Event totals">
      <div class="stat"><span class="stat-label">Events</span><strong class="stat-value">{{.TotalEvents}}</strong></div>
      <div class="stat"><span class="stat-label">Commands</span><strong class="stat-value">{{len .Rows}}</strong></div>
      <div class="stat"><span class="stat-label">Ignored lines</span><strong class="stat-value">{{.IgnoredLines}}</strong></div>
    </section>
    {{if .Rows}}
    <div class="table-shell">
      <table>
        <caption>Latency and error summary by command</caption>
        <thead><tr><th scope="col">Command</th><th scope="col">Count</th><th scope="col">Errors</th><th scope="col">Error rate</th><th scope="col">p50 ms</th><th scope="col">p95 ms</th><th scope="col">p99 ms</th><th scope="col">Max ms</th></tr></thead>
        <tbody>{{range .Rows}}<tr><td>{{.Command}}</td><td>{{.Count}}</td><td class="{{if .Errors}}error{{end}}">{{.Errors}}</td><td class="{{if .Errors}}error{{end}}">{{printf "%.2f%%" (mul100 .ErrorRate)}}</td><td>{{printf "%.2f" .Latency.P50}}</td><td>{{printf "%.2f" .Latency.P95}}</td><td>{{printf "%.2f" .Latency.P99}}</td><td>{{printf "%.2f" .Latency.Max}}</td></tr>{{end}}</tbody>
      </table>
    </div>
    {{else}}<p class="empty" role="status">No valid command events were found in the input.</p>{{end}}
    <footer>Generated at {{.GeneratedAt}}. Keep the source JSONL under the retention policy of the environment.</footer>
  </main>
</body>
</html>`))

// RenderDashboard writes a self-contained, escaped HTML report for a summary.
func RenderDashboard(output io.Writer, summary Summary) error {
	if output == nil {
		return io.ErrClosedPipe
	}
	rows := make([]dashboardRow, 0, len(summary.Commands))
	for command, value := range summary.Commands {
		rows = append(rows, dashboardRow{Command: command, Count: value.Count, Errors: value.Errors, ErrorRate: value.ErrorRate, Latency: value.LatencyMS})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Command < rows[j].Command })
	return dashboardTemplate.Execute(output, dashboardData{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		TotalEvents: summary.TotalEvents, IgnoredLines: summary.IgnoredLines, Rows: rows,
	})
}
