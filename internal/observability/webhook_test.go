package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestSendAlertWebhookSendsBoundedReport(t *testing.T) {
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			return nil, fmt.Errorf("unexpected webhook request: %s %s", r.Method, r.Header.Get("Content-Type"))
		}
		var report AlertReport
		if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
			return nil, err
		}
		if report.Event != "chitchat_cli_observability_alerts" || report.Healthy {
			return nil, fmt.Errorf("unexpected webhook report: %#v", report)
		}
		return &http.Response{StatusCode: http.StatusNoContent, Status: "204 No Content", Body: io.NopCloser(strings.NewReader(""))}, nil
	})

	err := sendAlertWebhookWithClient(context.Background(), "http://127.0.0.1/alerts", AlertReport{Event: "chitchat_cli_observability_alerts", Healthy: false}, &http.Client{Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSendAlertWebhookRejectsInsecureRemoteURL(t *testing.T) {
	err := SendAlertWebhook(context.Background(), "http://example.com/alerts", AlertReport{})
	if err == nil {
		t.Fatal("expected insecure remote webhook URL to be rejected")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
