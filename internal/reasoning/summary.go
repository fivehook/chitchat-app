package reasoning

import (
	"errors"
	"regexp"
	"strings"
)

const (
	MaxItems       = 16
	MaxItemLength  = 2000
	MaxTotalLength = 20000
)

type Summary struct {
	Assumptions []string `json:"assumptions"`
	Steps       []string `json:"steps"`
	Decisions   []string `json:"decisions"`
	Evidence    []string `json:"evidence"`
	Confidence  float64  `json:"confidence"`
}

var (
	bearerPattern = regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/=-]+`)
	secretPattern = regexp.MustCompile(`(?i)(api[_-]?key|access[_-]?token|password|secret|private[_-]?key)\s*[:=]\s*[^\s,;]+`)
	emailPattern  = regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`)
)

func Normalize(input Summary) (Summary, error) {
	if input.Confidence < 0 || input.Confidence > 1 {
		return Summary{}, errors.New("confidence must be between 0 and 1")
	}
	output := Summary{Confidence: input.Confidence}
	var err error
	if output.Assumptions, err = normalizeItems(input.Assumptions); err != nil {
		return Summary{}, errors.New("assumptions: " + err.Error())
	}
	if output.Steps, err = normalizeItems(input.Steps); err != nil {
		return Summary{}, errors.New("steps: " + err.Error())
	}
	if output.Decisions, err = normalizeItems(input.Decisions); err != nil {
		return Summary{}, errors.New("decisions: " + err.Error())
	}
	if output.Evidence, err = normalizeItems(input.Evidence); err != nil {
		return Summary{}, errors.New("evidence: " + err.Error())
	}
	if totalItemsLength(output) > MaxTotalLength {
		return Summary{}, errors.New("reasoning summary is too large")
	}
	return output, nil
}

func normalizeItems(items []string) ([]string, error) {
	if len(items) > MaxItems {
		return nil, errors.New("too many items")
	}
	output := make([]string, 0, len(items))
	for _, item := range items {
		item = redact(strings.TrimSpace(item))
		if item == "" || len(item) > MaxItemLength {
			return nil, errors.New("items must be non-empty and at most 2000 characters")
		}
		output = append(output, item)
	}
	return output, nil
}

func redact(value string) string {
	value = bearerPattern.ReplaceAllString(value, `${1}[REDACTED_TOKEN]`)
	value = secretPattern.ReplaceAllString(value, `${1}=[REDACTED_SECRET]`)
	return emailPattern.ReplaceAllString(value, "[REDACTED_EMAIL]")
}

func totalItemsLength(summary Summary) int {
	total := 0
	for _, items := range [][]string{summary.Assumptions, summary.Steps, summary.Decisions, summary.Evidence} {
		for _, item := range items {
			total += len(item)
		}
	}
	return total
}
