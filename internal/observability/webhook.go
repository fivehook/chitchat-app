package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxWebhookResponseBytes = 4096

// SendAlertWebhook sends only the bounded alert report. HTTPS is required for
// remote endpoints; plain HTTP is accepted only for loopback test/dev sinks.
func SendAlertWebhook(ctx context.Context, endpoint string, report AlertReport) error {
	return sendAlertWebhookWithClient(ctx, endpoint, report, &http.Client{Timeout: 10 * time.Second})
}

func sendAlertWebhookWithClient(ctx context.Context, endpoint string, report AlertReport, client *http.Client) error {
	if ctx == nil {
		return fmt.Errorf("webhook context is nil")
	}
	if client == nil {
		return fmt.Errorf("webhook client is nil")
	}
	u, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || u.Scheme == "" || u.Hostname() == "" || u.User != nil {
		return fmt.Errorf("webhook URL is invalid")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && isLoopbackHost(u.Hostname())) {
		return fmt.Errorf("webhook URL must use HTTPS unless it targets loopback")
	}
	payload, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("encode webhook report: %w", err)
	}
	requestContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestContext, http.MethodPost, u.String(), bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send webhook request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(response.Body, maxWebhookResponseBytes))
		return fmt.Errorf("webhook returned %s: %s", response.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
