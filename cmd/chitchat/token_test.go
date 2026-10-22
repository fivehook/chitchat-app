package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractAgentToken(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantArgs  []string
		wantToken string
		wantErr   bool
	}{
		{
			name:      "separate value",
			args:      []string{"get", "list", "--profile", "demo", "--token", "temporary"},
			wantArgs:  []string{"get", "list", "--profile", "demo"},
			wantToken: "temporary",
		},
		{
			name:      "equals value",
			args:      []string{"get", "list", "--token=temporary"},
			wantArgs:  []string{"get", "list"},
			wantToken: "temporary",
		},
		{
			name:    "missing value",
			args:    []string{"get", "list", "--token"},
			wantErr: true,
		},
		{
			name:    "empty equals value",
			args:    []string{"get", "list", "--token="},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args, token, err := extractAgentToken(test.args)
			if (err != nil) != test.wantErr {
				t.Fatalf("extractAgentToken() error = %v, wantErr %v", err, test.wantErr)
			}
			if test.wantErr {
				return
			}
			if token != test.wantToken {
				t.Fatalf("token = %q, want %q", token, test.wantToken)
			}
			if len(args) != len(test.wantArgs) {
				t.Fatalf("args = %#v, want %#v", args, test.wantArgs)
			}
			for index := range args {
				if args[index] != test.wantArgs[index] {
					t.Fatalf("args = %#v, want %#v", args, test.wantArgs)
				}
			}
		})
	}
}

func TestExchangeTokenUsesAgentContextToken(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("CHITCHAT_CONFIG", configPath)
	t.Setenv("CHITCHAT_AGENT_TOKEN", "agent-held-token")
	config := cliConfig{
		ActiveProfile: "demo",
		Profiles: map[string]cliProfile{
			"demo": {
				ProfileID:          "profile-id",
				ExchangeCredential: "exchange-credential",
				APIURL:             "http://invalid.local",
			},
		},
	}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	token, profile, err := exchangeToken("demo", "http://fallback.invalid")
	if err != nil {
		t.Fatal(err)
	}
	if token != "agent-held-token" {
		t.Fatalf("token = %q, want agent-held-token", token)
	}
	if profile.ProfileID != "profile-id" {
		t.Fatalf("profile id = %q, want profile-id", profile.ProfileID)
	}
}
