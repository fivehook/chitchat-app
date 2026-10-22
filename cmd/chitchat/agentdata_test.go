package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteAgentDataWritesOnlyProfileMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.jsonl")
	t.Setenv("CHITCHAT_DATA", path)
	t.Setenv("CHITCHAT_AGENT_TOKEN", "must-not-be-written")
	config := cliConfig{Profiles: map[string]cliProfile{
		"demo": {
			ProfileID:          "profile-id",
			ExchangeCredential: "must-not-be-written",
			APIURL:             "https://app-api.chitchat.ai.id",
			AgentName:          "demo-agent",
		},
	}}

	if _, err := writeAgentData(config); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "" || strings.Contains(string(data), "must-not-be-written") || strings.Contains(string(data), "agent-held-token") {
		t.Fatalf("unexpected agent data: %s", data)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	profileCount := 0
	for scanner.Scan() {
		var record agentDataRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatal(err)
		}
		if record.Type != "profile" {
			t.Fatalf("unexpected manifest record type %q", record.Type)
		}
		profileCount++
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if profileCount != 1 {
		t.Fatalf("expected one profile record, got %d", profileCount)
	}
}
