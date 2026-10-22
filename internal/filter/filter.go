package filter

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const DefaultTimeout = 500 * time.Millisecond

type PostDraft struct {
	Category        string
	Title           string
	ShortDesc       string
	Body            string
	ContentType     string
	Context         string
	ExpectedOutcome string
	Language        string
	Platform        string
	Model           string
	Tags            []string
	LicenseCode     string
	Attribution     string
	SourceURL       string
}

type PostFilter interface {
	Name() string
	ProcessDraft(context.Context, *PostDraft) (*PostDraft, error)
}

type Pipeline struct {
	filters []PostFilter
	timeout time.Duration
}

type Option func(*Pipeline)

func WithTimeout(timeout time.Duration) Option {
	return func(pipeline *Pipeline) {
		if timeout > 0 {
			pipeline.timeout = timeout
		}
	}
}

func NewPipeline(filters []PostFilter, options ...Option) *Pipeline {
	pipeline := &Pipeline{filters: append([]PostFilter(nil), filters...), timeout: DefaultTimeout}
	for _, option := range options {
		option(pipeline)
	}
	return pipeline
}

func DefaultPipeline(options ...Option) *Pipeline {
	return NewPipeline([]PostFilter{CredentialScanner{}}, options...)
}

func (pipeline *Pipeline) Apply(ctx context.Context, draft PostDraft) (PostDraft, error) {
	if pipeline == nil {
		return PostDraft{}, errors.New("privacy filter pipeline is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	current := cloneDraft(draft)
	for _, postFilter := range pipeline.filters {
		if postFilter == nil {
			continue
		}
		if err := ctx.Err(); err != nil {
			return PostDraft{}, err
		}
		filtered, err := pipeline.applyOne(ctx, postFilter, current)
		if err != nil {
			return PostDraft{}, fmt.Errorf("privacy filter %s: %w", postFilter.Name(), err)
		}
		if filtered == nil {
			return PostDraft{}, fmt.Errorf("privacy filter %s returned nil draft", postFilter.Name())
		}
		current = cloneDraft(*filtered)
	}
	return current, nil
}

func (pipeline *Pipeline) applyOne(ctx context.Context, postFilter PostFilter, draft PostDraft) (result *PostDraft, err error) {
	filterContext, cancel := context.WithTimeout(ctx, pipeline.timeout)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		result, err = postFilter.ProcessDraft(filterContext, &draft)
	}()
	select {
	case <-done:
		return result, err
	case <-filterContext.Done():
		return nil, filterContext.Err()
	}
}

type CredentialScanner struct{}

func (CredentialScanner) Name() string {
	return "credential-scanner"
}

func (CredentialScanner) ProcessDraft(ctx context.Context, draft *PostDraft) (*PostDraft, error) {
	if draft == nil {
		return nil, errors.New("draft is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	output := cloneDraft(*draft)
	output.Title = redact(output.Title)
	output.ShortDesc = redact(output.ShortDesc)
	output.Body = redact(output.Body)
	output.Context = redact(output.Context)
	output.ExpectedOutcome = redact(output.ExpectedOutcome)
	output.Attribution = redact(output.Attribution)
	output.SourceURL = redact(output.SourceURL)
	for index, tag := range output.Tags {
		output.Tags[index] = redact(tag)
	}
	return &output, nil
}

var credentialPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/=-]+`),
	regexp.MustCompile(`(?i)(api[_-]?key|access[_-]?token|password|secret|private[_-]?key|token)\s*[:=]\s*[^\s,;]+`),
	regexp.MustCompile(`(?i)\b(?:gh[pousr]|github_pat|glpat)-[A-Za-z0-9_\-]{20,}\b`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`(?s)-----BEGIN(?: [A-Z0-9]+)? PRIVATE KEY-----.*?-----END(?: [A-Z0-9]+)? PRIVATE KEY-----`),
}

func redact(value string) string {
	for _, pattern := range credentialPatterns {
		value = pattern.ReplaceAllStringFunc(value, func(match string) string {
			if strings.HasPrefix(strings.ToLower(match), "bearer") {
				return "Bearer [REDACTED_TOKEN]"
			}
			if strings.Contains(strings.ToLower(match), "private key") || strings.HasPrefix(match, "-----BEGIN") {
				return "[REDACTED_PRIVATE_KEY]"
			}
			return "[REDACTED_SECRET]"
		})
	}
	return value
}

func cloneDraft(draft PostDraft) PostDraft {
	draft.Tags = append([]string(nil), draft.Tags...)
	return draft
}
