package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const agentDataVersion = 1

type agentDataRecord struct {
	Version   int    `json:"version"`
	Type      string `json:"type"`
	Profile   string `json:"profile,omitempty"`
	ProfileID string `json:"profile_id,omitempty"`
	APIURL    string `json:"api_url,omitempty"`
	AgentName string `json:"agent_name,omitempty"`
	DirectAPI bool   `json:"direct_api"`
}

func agentDataFilePath() string {
	if value := strings.TrimSpace(os.Getenv("CHITCHAT_DATA")); value != "" {
		return value
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "chitchat", "data.jsonl")
	}
	return filepath.Join(home, ".chitchat", "data.jsonl")
}

func writeAgentData(config cliConfig) (string, error) {
	if len(config.Profiles) == 0 {
		return "", errors.New("cannot write agent data without a paired profile")
	}

	names := make([]string, 0, len(config.Profiles))
	for name := range config.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)

	records := make([]agentDataRecord, 0, len(names))
	for _, name := range names {
		profile := config.Profiles[name]
		records = append(records, agentDataRecord{
			Version:   agentDataVersion,
			Type:      "profile",
			Profile:   name,
			ProfileID: profile.ProfileID,
			APIURL:    profile.APIURL,
			AgentName: profile.AgentName,
			DirectAPI: false,
		})
	}

	path := agentDataFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return path, fmt.Errorf("create agent data directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".data-*.jsonl")
	if err != nil {
		return path, fmt.Errorf("create agent data file: %w", err)
	}
	tmpPath := tmp.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return path, fmt.Errorf("protect agent data file: %w", err)
	}
	writer := bufio.NewWriter(tmp)
	encoder := json.NewEncoder(writer)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			_ = tmp.Close()
			return path, fmt.Errorf("encode agent data record: %w", err)
		}
	}
	if err := writer.Flush(); err != nil {
		_ = tmp.Close()
		return path, fmt.Errorf("flush agent data file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return path, fmt.Errorf("close agent data file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return path, fmt.Errorf("save agent data file: %w", err)
	}
	removeTemp = false
	if err := os.Chmod(path, 0o600); err != nil {
		return path, fmt.Errorf("protect saved agent data file: %w", err)
	}
	return path, nil
}
