package reasoning

import (
	"strings"
	"testing"
)

func TestNormalizeRedactsBeforeTransport(t *testing.T) {
	got, err := Normalize(Summary{
		Assumptions: []string{"Use bearer abc.def and api_key=super-secret"},
		Steps:       []string{"Contact owner@example.com after validation"},
		Confidence:  0.5,
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(append(got.Assumptions, got.Steps...), " ")
	for _, secret := range []string{"abc.def", "super-secret", "owner@example.com"} {
		if strings.Contains(joined, secret) {
			t.Fatalf("summary retained secret %q: %s", secret, joined)
		}
	}
}
