package observability

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderDashboardEscapesCommandAndIncludesMetrics(t *testing.T) {
	var output bytes.Buffer
	summary := Summary{
		TotalEvents: 2,
		Commands: map[string]CommandSummary{
			"get.<script>": {Count: 2, Errors: 1, ErrorRate: 0.5, LatencyMS: LatencySummary{P50: 10, P95: 20, P99: 25, Max: 30}},
		},
	}
	if err := RenderDashboard(&output, summary); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{"Chitchat CLI observability", "50.00%", "p95 ms", "get.&lt;script&gt;"} {
		if !strings.Contains(html, want) {
			t.Fatalf("dashboard missing %q: %s", want, html)
		}
	}
	if strings.Contains(html, "get.<script>") {
		t.Fatal("dashboard rendered an unescaped command")
	}
}
