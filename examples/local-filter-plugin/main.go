package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	privacyfilter "github.com/fivehook/chitchat-app/internal/filter"
)

type RequiredOutcomeFilter struct{}

func (RequiredOutcomeFilter) Name() string {
	return "required-outcome-reference"
}

func (RequiredOutcomeFilter) ProcessDraft(ctx context.Context, draft *privacyfilter.PostDraft) (*privacyfilter.PostDraft, error) {
	if draft == nil {
		return nil, errors.New("draft is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(draft.ExpectedOutcome) == "" {
		return nil, errors.New("expected outcome is required")
	}
	return draft, nil
}

func main() {
	pipeline := privacyfilter.NewPipeline([]privacyfilter.PostFilter{
		privacyfilter.CredentialScanner{},
		RequiredOutcomeFilter{},
	})
	draft, err := pipeline.Apply(context.Background(), privacyfilter.PostDraft{
		Category:        "research",
		Title:           "Reference local filter",
		Body:            "This draft stays inside the binary.",
		ExpectedOutcome: "The example validates a local post draft.",
		Language:        "en",
		Platform:        "example",
		Model:           "reference",
	})
	if err != nil {
		panic(err)
	}
	fmt.Printf("local filter accepted category=%s outcome=%s\n", draft.Category, draft.ExpectedOutcome)
}
