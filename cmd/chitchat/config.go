package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const defaultAPIURL = "https://app-api.chitchat.ai.id"

type cliConfig struct {
	ActiveProfile string                `json:"active_profile,omitempty"`
	Profiles      map[string]cliProfile `json:"profiles"`
}

type cliProfile struct {
	ProfileID          string `json:"profile_id"`
	ExchangeCredential string `json:"exchange_credential"`
	APIURL             string `json:"api_url"`
	AgentName          string `json:"agent_name"`
	CourierCursor      int64  `json:"courier_cursor,omitempty"`
}

func configFilePath() string {
	if value := os.Getenv("CHITCHAT_CONFIG"); value != "" {
		return value
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "chitchat", "config.json")
	}
	return filepath.Join(base, "chitchat", "config.json")
}

func loadCLIConfig() (cliConfig, error) {
	config := cliConfig{Profiles: map[string]cliProfile{}}
	data, err := os.ReadFile(configFilePath())
	if errors.Is(err, os.ErrNotExist) {
		return config, nil
	}
	if err != nil {
		return config, err
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return config, err
	}
	if config.Profiles == nil {
		config.Profiles = map[string]cliProfile{}
	}
	return config, nil
}

func saveCLIConfig(config cliConfig) error {
	path := configFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func saveCourierCursor(profileName string, cursor int64) error {
	config, err := loadCLIConfig()
	if err != nil {
		return err
	}
	profile, ok := config.Profiles[profileName]
	if !ok {
		return errors.New("configured profile was removed while saving courier cursor")
	}
	if cursor <= profile.CourierCursor {
		return nil
	}
	profile.CourierCursor = cursor
	config.Profiles[profileName] = profile
	return saveCLIConfig(config)
}

func configuredProfile(name string) (cliProfile, string, error) {
	config, err := loadCLIConfig()
	if err != nil {
		return cliProfile{}, "", err
	}
	if name == "" {
		name = config.ActiveProfile
	}
	if name == "" {
		return cliProfile{}, "", errors.New("profile is required via --profile or chitchat profile use")
	}
	profile, ok := config.Profiles[name]
	if !ok || profile.ExchangeCredential == "" || profile.ProfileID == "" {
		return cliProfile{}, "", errors.New("configured profile is missing exchange credential or profile id")
	}
	return profile, name, nil
}

func publicAPIURL() *string {
	apiURL := defaultAPIURL
	return &apiURL
}
