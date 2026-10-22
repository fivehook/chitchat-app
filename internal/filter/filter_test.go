package filter

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestCredentialScannerRedactsAllPostFields(t *testing.T) {
	draft := PostDraft{
		Title:           "Bearer abc.def title",
		ShortDesc:       "api_key=top-secret",
		Body:            "github_pat-abcdefghijklmnopqrstuvwxyz and AKIA1234567890ABCDEF",
		Context:         "password: hunter2",
		ExpectedOutcome: "-----BEGIN PRIVATE KEY-----secret-----END PRIVATE KEY-----",
		Attribution:     "secret=attribution-secret",
		SourceURL:       "https://example.test/?access_token=abc123",
		Tags:            []string{"safe", "token=tag-secret"},
	}

	filtered, err := DefaultPipeline().Apply(context.Background(), draft)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join([]string{
		filtered.Title, filtered.ShortDesc, filtered.Body, filtered.Context,
		filtered.ExpectedOutcome, filtered.Attribution, filtered.SourceURL,
		strings.Join(filtered.Tags, " "),
	}, "\n")
	for _, secret := range []string{"abc.def", "top-secret", "abcdefghijklmnopqrstuvwxyz", "AKIA1234567890ABCDEF", "hunter2", "attribution-secret", "abc123", "tag-secret"} {
		if strings.Contains(joined, secret) {
			t.Fatalf("secret %q remained in filtered draft: %s", secret, joined)
		}
	}
	if !strings.Contains(joined, "REDACTED") {
		t.Fatalf("expected redaction marker in filtered draft: %s", joined)
	}
}

func TestPipelineTimesOutUncooperativeFilter(t *testing.T) {
	pipeline := NewPipeline([]PostFilter{blockingFilter{}}, WithTimeout(5*time.Millisecond))
	_, err := pipeline.Apply(context.Background(), PostDraft{Body: "body"})
	if err == nil || !strings.Contains(err.Error(), "deadline exceeded") {
		t.Fatalf("expected timeout, got %v", err)
	}
}

type blockingFilter struct{}

func (blockingFilter) Name() string {
	return "blocking-test-filter"
}

func (blockingFilter) ProcessDraft(context.Context, *PostDraft) (*PostDraft, error) {
	time.Sleep(100 * time.Millisecond)
	return &PostDraft{Body: "late"}, nil
}
