package main

import (
	"errors"
	"flag"
	"os"
	"strings"
	"time"
)

const agentTokenTTL = 5 * time.Minute

// extractAgentToken keeps the temporary token out of command-specific flag
// sets. It is installed only in the current process environment so nested MCP
// commands can inherit it without writing it to disk.
func extractAgentToken(args []string) ([]string, string, error) {
	clean := make([]string, 0, len(args))
	var token string
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "--token":
			if index+1 >= len(args) {
				return nil, "", errors.New("--token requires a value")
			}
			index++
			token = strings.TrimSpace(args[index])
			if token == "" {
				return nil, "", errors.New("--token cannot be empty")
			}
		case strings.HasPrefix(arg, "--token="):
			token = strings.TrimSpace(strings.TrimPrefix(arg, "--token="))
			if token == "" {
				return nil, "", errors.New("--token cannot be empty")
			}
		default:
			clean = append(clean, arg)
		}
	}
	return clean, token, nil
}

func exchangeAgentToken(args []string, stdout, stderr *os.File) error {
	flags := flag.NewFlagSet("token exchange", flag.ContinueOnError)
	flags.SetOutput(stderr)
	profileName := flags.String("profile", envOr("CHITCHAT_PROFILE", ""), "configured profile")
	apiURL := publicAPIURL()
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("token exchange does not accept positional arguments")
	}
	resolvedProfile := strings.TrimSpace(*profileName)
	var err error
	if resolvedProfile == "" {
		_, resolvedProfile, err = configuredProfile("")
		if err != nil {
			return err
		}
	}
	token, profile, err := exchangeTokenFresh(resolvedProfile, *apiURL)
	if err != nil {
		return err
	}
	return writeJSON(stdout, map[string]any{
		"profile":    resolvedProfile,
		"selection":  "active profile selected internally by the CLI; pair code chose it",
		"profile_id": profile.ProfileID,
		"token":      token,
		"expires_in": int(agentTokenTTL / time.Second),
		"expires_at": timeNow().Add(agentTokenTTL),
		"storage":    "agent-context-only",
		"persisted":  false,
		"direct_api": false,
		"next_step":  "Pass this token to subsequent chitchat commands with --token. Do not read config, write, print again, persist, or share it.",
	})
}
