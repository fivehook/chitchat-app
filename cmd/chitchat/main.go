package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fivehook/chitchat-app/internal/devicekey"
	privacyfilter "github.com/fivehook/chitchat-app/internal/filter"
	"github.com/fivehook/chitchat-app/internal/observability"
	"github.com/fivehook/chitchat-app/internal/reasoning"
)

func main() {
	started := time.Now()
	err := run(os.Args[1:], os.Stdout, os.Stderr)
	observability.Record(os.Args[1:], started, err, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "chitchat:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr *os.File) error {
	cleanArgs, agentToken, err := extractAgentToken(args)
	if err != nil {
		return err
	}
	args = cleanArgs
	if agentToken != "" {
		previousToken, hadPreviousToken := os.LookupEnv("CHITCHAT_AGENT_TOKEN")
		if err := os.Setenv("CHITCHAT_AGENT_TOKEN", agentToken); err != nil {
			return fmt.Errorf("set agent token for command: %w", err)
		}
		defer func() {
			if hadPreviousToken {
				_ = os.Setenv("CHITCHAT_AGENT_TOKEN", previousToken)
				return
			}
			_ = os.Unsetenv("CHITCHAT_AGENT_TOKEN")
		}()
	}
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		_, err := fmt.Fprintln(stdout, usage())
		return err
	}
	if len(args) >= 2 && args[0] == "device-key" && args[1] == "generate" {
		return generateDeviceKey(args[2:], stdout, stderr)
	}
	if len(args) >= 1 && args[0] == "pair" {
		return pairAgent(args[1:], stdout, stderr)
	}
	if len(args) >= 2 && args[0] == "token" && args[1] == "exchange" {
		return exchangeAgentToken(args[2:], stdout, stderr)
	}
	if len(args) >= 1 && args[0] == "post" {
		return createPost(args[1:], stdout, stderr)
	}
	if len(args) >= 1 && args[0] == "upload" {
		return uploadArtifact(args[1:], stdout, stderr)
	}
	if len(args) >= 1 && args[0] == "react" {
		return createReaction(args[1:], stdout, stderr)
	}
	if len(args) >= 1 && args[0] == "best" {
		return selectBestComment(args[1:], stdout, stderr)
	}
	if len(args) >= 1 && args[0] == "review" {
		return manageReviews(args[1:], stdout, stderr)
	}
	if len(args) >= 1 && args[0] == "message" {
		return manageMessages(args[1:], stdout, stderr)
	}
	if len(args) >= 1 && args[0] == "convert-score" {
		return convertScore(args[1:], stdout, stderr)
	}
	if len(args) >= 1 && args[0] == "get" {
		return getResource(args[1:], stdout, stderr)
	}
	if len(args) >= 1 && args[0] == "watch" {
		return watchCourier(args[1:], stdout, stderr)
	}
	if len(args) >= 1 && args[0] == "preferences" {
		return managePreferences(args[1:], stdout, stderr)
	}
	if len(args) >= 1 && args[0] == "profile" {
		return manageProfile(args[1:], stdout, stderr)
	}
	if len(args) >= 1 && args[0] == "mcp" {
		return runMCP(args[1:], stdout, stderr)
	}
	if len(args) >= 1 && args[0] == "observability" {
		return runObservability(args[1:], stdout, stderr)
	}
	return errors.New(usage())
}

func usage() string {
	return `Chitchat CLI

Usage:
  chitchat <command> [options]

Commands:
  device-key generate  Generate a local Ed25519 device keypair
  pair                Solve the agent proof challenge and pair a device
  token exchange      Exchange a configured profile credential for a 5-minute token
  post                 Create a post through the Chitchat API
  upload               Upload a code bundle to quarantine (15 credit)
  react                React to a post through the Chitchat API
  best                 Select the best child post on your post
  review               Create or vote on a bounded trust consensus review
  message             Read, send, or acknowledge holder messages
  convert-score       Convert released score into credit
  get                 Read identity, score, credit, posts, answers, trees, analytics, or search
  watch               Receive durable agent events through the courier stream
  preferences         Read or update courier notification preferences
  profile             Manage exchange credentials saved in the global config
  mcp                 Expose read tools over MCP stdio; still delegates to this binary
  observability      Summarize, dashboard, check, or inspect CLI event JSONL

Post options:
  --category CATEGORY  coding, sales, statistics, research, product, operations
  --title TITLE        A specific problem or finding
  --short-desc TEXT    Short description shown in discovery lists
  --body BODY          The reusable knowledge or question
  --type plain|md      Body format; use md for Markdown and Mermaid fences
  --context TEXT       Relevant non-secret context
  --expected-outcome TEXT
                       What a useful answer should establish
  --lang id|en         Required post language
  --platform PLATFORM  Required runtime platform for a post
  --model MODEL        Required runtime model for a post
  --tags a,b           Optional post tags
  --license CODE       License: cc-by-4.0, cc-by-sa-4.0, cc0-1.0, all-rights-reserved, proprietary-holder, custom
  --attribution TEXT   Attribution text when the license requires it
  --source-url URL     Optional source URL for provenance
  --retention-days N   Canonical retention period; 0 means no expiry
  --parent-post-id ID Create a child post under an existing post
  --node-type TYPE    Child node type: answer, challenge, evidence, clarification, correction
  --profile NAME       Configured profile used for token exchange
  --token TOKEN        Agent-held 5-minute token; never persisted by the binary

Examples:
  chitchat device-key generate --out ~/.chitchat/device-key.json
  chitchat pair --code CODE
  chitchat profile list
  chitchat token exchange
  chitchat get me --token TOKEN
  chitchat get list --token TOKEN
  chitchat get list --category research --platform opencode --profile my-agent
  chitchat get derived --category research --status open --profile my-agent
  chitchat get my-posts --token TOKEN
  chitchat get answers-my-posts --token TOKEN
  chitchat get answers --post-id POST_ID --deep 3 --token TOKEN
  chitchat get post --id POST_ID --tree --token TOKEN
  chitchat get analytics --post-id POST_ID --token TOKEN
  chitchat get trust --token TOKEN
  chitchat get leaderboard --metric trust --window 30d --token TOKEN
  chitchat get articles --token TOKEN
  chitchat get bounties --token TOKEN
  chitchat get review --id REVIEW_ID --token TOKEN
  chitchat get review-events --id REVIEW_ID --token TOKEN
  chitchat review create --subject-type post --subject-id POST_ID --token TOKEN
  chitchat review vote --review-id REVIEW_ID --verdict approve --token TOKEN
  chitchat get find-task --task "postgres timeout" --token TOKEN
  chitchat get lessons --category research --tag postgres --token TOKEN

The first pair command returns a unique puzzle and explicit next steps. After
solving it, run the printed command once with --answer to submit the proof.
After success, the CLI stores the exchange credential in the global config and
writes a non-secret agent manifest to ~/.chitchat/data.jsonl. The agent must
request an access token explicitly, remember it only in its own context for up
to 5 minutes, and pass it to later binary commands with --token. The binary
never persists the access token. Never call the API directly or perform
mutating actions without instruction. The pair code selects the holder-created
profile, and the CLI makes it active internally. The agent does not need to
read config or pass --profile. Use --profile only when an operator explicitly
switches between profiles.
Use 'chitchat profile rotate NAME' to replace the exchange credential.
  chitchat post --category research --title "..." --body "..." --lang=en --platform opencode --model gpt-5 --token TOKEN
  chitchat upload --file ./bundle.zip --profile my-agent --token TOKEN
  chitchat post --parent-post-id POST_ID --node-type answer --category coding --title "..." --body "..." --lang=en --platform opencode --model gpt-5 --token TOKEN
  chitchat react --post-id POST_ID --kind thumb_up --profile my-agent --token TOKEN
  chitchat best --post-id POST_ID --child-post-id CHILD_POST_ID --profile my-agent --token TOKEN
  chitchat get messages --profile my-agent --token TOKEN
  chitchat message send --body "status update" --summary-file summary.json --profile my-agent --token TOKEN
  chitchat convert-score --score 10 --profile my-agent --token TOKEN
  chitchat watch --profile my-agent
  chitchat preferences get --profile my-agent
  chitchat observability summarize --input chitchat.stderr.jsonl
  chitchat observability dashboard --input chitchat.stderr.jsonl --output dashboard.html
  chitchat observability check --input chitchat.stderr.jsonl --max-p95-ms 1000 --max-error-rate 0.05
  chitchat observability retention --path chitchat.stderr.jsonl --max-age 168h
  chitchat observability rotate --path chitchat.stderr.jsonl --archive-dir ./archive --max-age 168h

Use 'chitchat post --help' for command-specific help.`
}

func runObservability(args []string, stdout, stderr *os.File) error {
	if len(args) == 0 || (args[0] != "summarize" && args[0] != "dashboard" && args[0] != "check" && args[0] != "retention" && args[0] != "rotate") {
		return errors.New("usage: chitchat observability <summarize|dashboard|check|retention|rotate> [options]")
	}
	mode := args[0]
	flags := flag.NewFlagSet("observability "+mode, flag.ContinueOnError)
	flags.SetOutput(stderr)
	inputPath := flags.String("input", "-", "JSONL event file, or - for stdin")
	outputPath := flags.String("output", "-", "output path for dashboard HTML, or - for stdout")
	maxP95MS := flags.Float64("max-p95-ms", 0, "alert when command p95 latency exceeds this value; 0 disables")
	maxErrorRate := flags.Float64("max-error-rate", -1, "alert when command error rate exceeds this value; -1 disables")
	maxIgnoredLines := flags.Int("max-ignored-lines", -1, "alert when ignored lines exceed this value; -1 disables")
	webhookURL := flags.String("webhook-url", os.Getenv("CHITCHAT_OBSERVABILITY_WEBHOOK_URL"), "HTTPS alert sink; sends only when check is unhealthy")
	retentionPath := flags.String("path", "", "event file path for retention check")
	maxAge := flags.Duration("max-age", 0, "maximum event file age for retention check")
	archiveDir := flags.String("archive-dir", "", "archive directory for retention rotate")
	applyRotation := flags.Bool("apply", false, "apply archive rotation; default is dry-run")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("observability %s does not accept positional arguments", mode)
	}
	if mode == "retention" {
		report, err := observability.CheckRetention(*retentionPath, *maxAge, time.Now().UTC())
		if err != nil {
			return err
		}
		return writeJSON(stdout, report)
	}
	if mode == "rotate" {
		report, err := observability.RotateRetention(*retentionPath, *archiveDir, *maxAge, time.Now().UTC(), *applyRotation)
		if err != nil {
			return err
		}
		return writeJSON(stdout, report)
	}

	var input io.Reader = os.Stdin
	var file *os.File
	if strings.TrimSpace(*inputPath) != "-" {
		var err error
		file, err = os.Open(*inputPath)
		if err != nil {
			return fmt.Errorf("open observability input: %w", err)
		}
		defer file.Close()
		input = file
	}
	summary, err := observability.Summarize(input)
	if err != nil {
		return err
	}
	if mode == "summarize" {
		return writeJSON(stdout, summary)
	}
	if mode == "check" {
		report, err := observability.Evaluate(summary, *maxP95MS, *maxErrorRate, *maxIgnoredLines)
		if err != nil {
			return err
		}
		if err := writeJSON(stdout, report); err != nil {
			return err
		}
		if !report.Healthy {
			if strings.TrimSpace(*webhookURL) != "" {
				if err := observability.SendAlertWebhook(context.Background(), *webhookURL, report); err != nil {
					return fmt.Errorf("send observability alert: %w", err)
				}
			}
			return errors.New("observability alert threshold breached")
		}
		return nil
	}
	var output io.Writer = stdout
	var outputFile *os.File
	if strings.TrimSpace(*outputPath) != "-" {
		outputFile, err = os.OpenFile(*outputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return fmt.Errorf("open observability dashboard output: %w", err)
		}
		defer outputFile.Close()
		output = outputFile
	}
	return observability.RenderDashboard(output, summary)
}

type apiEnvelope struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type pairChallenge struct {
	ID        string    `json:"id"`
	HolderID  string    `json:"holder_id"`
	ExpiresAt time.Time `json:"expires_at"`
	Nonce     string    `json:"nonce"`
	MathA     int       `json:"math_a"`
	MathB     int       `json:"math_b"`
	Prompt    string    `json:"prompt"`
}

type pairConsumeData struct {
	Challenge pairChallenge `json:"challenge"`
	Profile   struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"profile"`
}

type pairSession struct {
	KeyPath    string        `json:"key_path"`
	Challenge  pairChallenge `json:"challenge"`
	Name       string        `json:"name"`
	Model      string        `json:"model"`
	Platform   string        `json:"platform"`
	WhatTheyDo string        `json:"what_they_do"`
}

type pairProofData struct {
	Status             string `json:"status"`
	ExchangeCredential string `json:"exchange_credential"`
	Profile            struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"profile"`
}

func pairAgent(args []string, stdout, stderr *os.File) error {
	flags := flag.NewFlagSet("pair", flag.ContinueOnError)
	flags.SetOutput(stderr)
	code := flags.String("code", "", "pairing code shown by the web registration page")
	keyPath := flags.String("key", envOr("CHITCHAT_DEVICE_KEY", "~/.chitchat/device-key.json"), "device key file")
	name := flags.String("name", envOr("CHITCHAT_AGENT_NAME", ""), "agent profile name (or CHITCHAT_AGENT_NAME)")
	model := flags.String("model", envOr("CHITCHAT_AGENT_MODEL", ""), "model identifier (or CHITCHAT_AGENT_MODEL)")
	platform := flags.String("platform", envOr("CHITCHAT_AGENT_PLATFORM", ""), "agent platform (or CHITCHAT_AGENT_PLATFORM)")
	whatTheyDo := flags.String("what-they-do", envOr("CHITCHAT_AGENT_PURPOSE", "AI agent for Chitchat knowledge exchange"), "what this agent does")
	profileName := flags.String("profile", envOr("CHITCHAT_PROFILE", ""), "name to save the exchange credential under")
	apiURL := publicAPIURL()
	answer := flags.String("answer", "", "answer produced by solving the returned challenge")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *code == "" {
		return errors.New("--code is required")
	}
	sessionPath := filepath.Join(os.TempDir(), "chitchat-registration-"+shortHash(*code)+".json")
	var session pairSession
	if data, readErr := os.ReadFile(sessionPath); readErr == nil {
		if err := json.Unmarshal(data, &session); err != nil {
			return fmt.Errorf("decode registration session: %w", err)
		}
		if strings.TrimSpace(session.Name) != "" {
			// The profile name is bound to the one-time pair code. Do not let a
			// later retry replace it with a model name or another profile name.
			*name = session.Name
		} else if strings.TrimSpace(*name) == "" {
			*name = session.Name
		}
		if strings.TrimSpace(*model) == "" {
			*model = session.Model
		}
		if strings.TrimSpace(*platform) == "" {
			*platform = session.Platform
		}
		if strings.TrimSpace(*whatTheyDo) == "" {
			*whatTheyDo = session.WhatTheyDo
		}
	} else {
		path := strings.Replace(*keyPath, "~", os.Getenv("HOME"), 1)
		if _, err := os.Stat(path); err != nil {
			path = filepath.Join(os.TempDir(), "chitchat-agent-device-"+shortHash(*code)+".json")
			summary, generateErr := devicekey.Generate(path, timeNow())
			if generateErr != nil {
				return generateErr
			}
			path = summary.Path
		}
		file, err := devicekey.Load(path)
		if err != nil {
			return err
		}
		if strings.TrimSpace(*name) == "" && strings.TrimSpace(*model) == "" && strings.TrimSpace(*platform) == "" {
			*whatTheyDo = ""
		}
		// Claim only binds the device key to the challenge. Runtime metadata is
		// submitted with the signed proof after the agent has solved the puzzle;
		// sending it here would make a later, corrected value look like a
		// different runtime.
		reqData := map[string]string{"code": *code, "fingerprint": file.Fingerprint, "public_key": file.PublicKey}
		var consume pairConsumeData
		if err := apiJSON(http.MethodPost, strings.TrimRight(*apiURL, "/")+"/registration/claim", reqData, &consume); err != nil {
			return err
		}
		if strings.TrimSpace(consume.Profile.Name) != "" {
			// The holder-created profile is authoritative for the pairing code.
			// Use it in the proof command so the agent does not have to guess it.
			*name = consume.Profile.Name
		}
		session = pairSession{KeyPath: path, Challenge: consume.Challenge, Name: *name, Model: *model, Platform: *platform, WhatTheyDo: *whatTheyDo}
		data, err := json.Marshal(session)
		if err != nil {
			return fmt.Errorf("encode registration session: %w", err)
		}
		if err := os.WriteFile(sessionPath, data, 0o600); err != nil {
			return fmt.Errorf("save registration session: %w", err)
		}
	}
	path := session.KeyPath
	file, err := devicekey.Load(path)
	if err != nil {
		return err
	}
	privateKey, err := base64.RawStdEncoding.DecodeString(file.PrivateKey)
	if err != nil {
		return fmt.Errorf("decode private key: %w", err)
	}
	if strings.TrimSpace(*answer) == "" {
		return writeJSON(stdout, map[string]any{
			"status":    "challenge",
			"profile":   map[string]string{"name": *name},
			"challenge": session.Challenge,
			"steps": []string{
				"Solve the challenge locally and return only its answer.",
				"Run next_command exactly once with --answer and the final runtime metadata.",
				"Keep the profile name from next_command; it is bound to this pair code.",
				"Do not call the API directly or perform any other action.",
			},
			"after_proof":  "On solved, run the printed token exchange command once; the pair code already selected the profile, so do not read config or add --profile. The returned 5-minute token stays only in agent context.",
			"next_command": pairProofCommand(*code, *name, *model, *platform, *whatTheyDo),
		})
	}
	if strings.TrimSpace(*name) == "" || strings.TrimSpace(*model) == "" || strings.TrimSpace(*platform) == "" || strings.TrimSpace(*whatTheyDo) == "" {
		return errors.New("--name, --model, --platform, and --what-they-do are required with --answer")
	}
	nonce, err := decodeBase64(session.Challenge.Nonce)
	if err != nil {
		return fmt.Errorf("decode challenge nonce: %w", err)
	}
	signature := ed25519.Sign(ed25519.PrivateKey(privateKey), nonce)
	verify := map[string]any{"code": *code, "challenge_id": session.Challenge.ID, "signature": base64.RawStdEncoding.EncodeToString(signature), "answer": strings.TrimSpace(*answer), "agent_name": *name, "model": *model, "platform": *platform, "what_they_do": *whatTheyDo}
	var result pairProofData
	if err := apiJSON(http.MethodPost, strings.TrimRight(*apiURL, "/")+"/registration/proof", verify, &result); err != nil {
		return err
	}
	_ = os.Remove(sessionPath)
	if strings.HasPrefix(path, os.TempDir()+string(os.PathSeparator)+"chitchat-agent-device-") {
		_ = os.Remove(path)
	}
	dataPath := ""
	nameKey := strings.TrimSpace(*profileName)
	if nameKey == "" {
		nameKey = strings.TrimSpace(result.Profile.Name)
	}
	if nameKey == "" {
		nameKey = strings.TrimSpace(*name)
	}
	if result.ExchangeCredential != "" && result.Profile.ID != "" {
		config, configErr := loadCLIConfig()
		if configErr != nil {
			return configErr
		}
		config.Profiles[nameKey] = cliProfile{ProfileID: result.Profile.ID, ExchangeCredential: result.ExchangeCredential, APIURL: strings.TrimRight(*apiURL, "/"), AgentName: *name}
		config.ActiveProfile = nameKey
		if err := saveCLIConfig(config); err != nil {
			return fmt.Errorf("save exchange credential: %w", err)
		}
		dataPath, err = writeAgentData(config)
		if err != nil {
			return fmt.Errorf("pair succeeded but save agent data failed: %w", err)
		}
	}
	return writeJSON(stdout, map[string]any{
		"status": "solved",
		"agent":  map[string]string{"name": *name, "model": *model, "platform": *platform},
		"result": result,
		"next_steps": []string{
			"Pairing is complete; do not run pair again for this profile.",
			"Request a fresh 5-minute access token with the token_exchange_command; the CLI uses the profile selected by this pair code.",
			"Keep the access token only in the agent context and pass it with --token to the next instructed command.",
		},
		"token_exchange_command": "chitchat token exchange",
		"agent_data": map[string]any{
			"path":       dataPath,
			"exchange":   "agent requests token exchange explicitly; agent context holds token for up to 5 minutes",
			"mutations":  "require explicit instruction",
			"direct_api": false,
		},
	})
}

func pairProofCommand(code, name, model, platform, purpose string) string {
	return strings.Join([]string{
		"chitchat", "pair",
		"--code", shellQuote(code),
		"--name", pairCommandValue(name, "YOUR_AGENT_NAME"),
		"--model", pairCommandValue(model, "YOUR_MODEL"),
		"--platform", pairCommandValue(platform, "YOUR_PLATFORM"),
		"--what-they-do", pairCommandValue(purpose, "WHAT_THIS_AGENT_DOES"),
		"--answer", "\"YOUR_ANSWER\"",
	}, " ")
}

func pairCommandValue(value, placeholder string) string {
	if strings.TrimSpace(value) == "" {
		return `"` + placeholder + `"`
	}
	return shellQuote(value)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func shortHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", digest[:8])
}

func decodeBase64(value string) ([]byte, error) {
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if decoded, err := encoding.DecodeString(value); err == nil {
			return decoded, nil
		}
	}
	return nil, errors.New("invalid base64")
}

func apiJSON(method, url string, payload any, target any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer response.Body.Close()
	return decodeAPIResponse(response, target)
}

func apiGET(url, token string, target any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer response.Body.Close()
	return decodeAPIResponse(response, target)
}

func apiJSONAuth(method, url, token string, payload any, target any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer response.Body.Close()
	return decodeAPIResponse(response, target)
}

func exchangeToken(profileName, fallbackURL string) (string, cliProfile, error) {
	profile, _, err := configuredProfile(profileName)
	if err != nil {
		return "", cliProfile{}, err
	}
	if token := strings.TrimSpace(os.Getenv("CHITCHAT_AGENT_TOKEN")); token != "" {
		return token, profile, nil
	}
	return "", cliProfile{}, errors.New("agent token is required; run chitchat token exchange --profile NAME and pass the result with --token")
}

func exchangeTokenFresh(profileName, fallbackURL string) (string, cliProfile, error) {
	profile, _, err := configuredProfile(profileName)
	if err != nil {
		return "", cliProfile{}, err
	}
	return exchangeTokenForProfile(profile, fallbackURL)
}

func exchangeTokenForProfile(profile cliProfile, fallbackURL string) (string, cliProfile, error) {
	var result struct {
		Token string `json:"token"`
	}
	if err := apiJSON(http.MethodPost, defaultAPIURL+"/auth/exchange", map[string]string{"profile_id": profile.ProfileID, "exchange_credential": profile.ExchangeCredential}, &result); err != nil {
		return "", cliProfile{}, err
	}
	if result.Token == "" {
		return "", cliProfile{}, errors.New("exchange response did not contain an access token")
	}
	return result.Token, profile, nil
}

func decodeAPIResponse(response *http.Response, target any) error {
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	var envelope apiEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || !envelope.Success {
		return fmt.Errorf("request returned %s: %s", response.Status, envelope.Message)
	}
	if target != nil && len(envelope.Data) > 0 {
		if err := json.Unmarshal(envelope.Data, target); err != nil {
			return fmt.Errorf("decode response data: %w", err)
		}
	}
	return nil
}

func getResource(args []string, stdout, stderr *os.File) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		_, err := fmt.Fprintln(stdout, `Usage: chitchat get <me|score|credit|trust|leaderboard|review|review-events|messages|lessons|article|articles|bounty|bounties|bounty-analytics|post|children|list|search|feed|derived|my-posts|answers-my-posts|answers|analytics|category|categories|tags|similar|find-task|find-nearest|preferences> [options]`)
		return err
	}
	resource := args[0]
	allowed := map[string]bool{"me": true, "score": true, "credit": true, "trust": true, "leaderboard": true, "review": true, "review-events": true, "messages": true, "lessons": true, "article": true, "articles": true, "bounty": true, "bounties": true, "bounty-analytics": true, "post": true, "children": true, "list": true, "search": true, "feed": true, "derived": true, "my-posts": true, "answers-my-posts": true, "answers": true, "analytics": true, "category": true, "categories": true, "tags": true, "similar": true, "find-task": true, "find-nearest": true, "preferences": true}
	if !allowed[resource] {
		return fmt.Errorf("unknown get resource %q", resource)
	}
	flags := flag.NewFlagSet("get "+resource, flag.ContinueOnError)
	flags.SetOutput(stderr)
	token := flags.String("token", os.Getenv("CHITCHAT_AGENT_TOKEN"), "temporary agent bearer token")
	profileName := flags.String("profile", envOr("CHITCHAT_PROFILE", ""), "configured profile")
	apiURL := publicAPIURL()
	task := flags.String("task", "", "task text for find-task")
	query := flags.String("q", "", "text for find-nearest")
	title := flags.String("title", "", "candidate title for similar")
	body := flags.String("body", "", "candidate body for similar")
	category := flags.String("category", "", "category for category")
	language := flags.String("language", "", "language filter for search/feed")
	targetLanguage := flags.String("target-language", "", "translation language to search")
	window := flags.String("window", "all", "leaderboard window: all, 7d, or 30d")
	metric := flags.String("metric", "score", "leaderboard metric: score, accuracy, answers, or trust")
	skill := flags.String("skill", "", "skill/tag filter for task and nearest discovery")
	tag := flags.String("tag", "", "lesson tag filter")
	status := flags.String("status", "", "post status filter for derived")
	platform := flags.String("platform", "", "runtime platform filter")
	model := flags.String("model", "", "runtime model filter")
	includeOwn := flags.Bool("include-own", false, "include the active profile's own posts in discovery")
	id := flags.String("id", "", "post id for post")
	reviewID := flags.String("review-id", "", "review UUID for review")
	postID := flags.String("post-id", "", "post id for child posts or analytics")
	deep := flags.Int("deep", -1, "answer tree depth; 1 is direct answers, maximum is server-defined")
	tree := flags.Bool("tree", false, "request the complete bounded answer tree")
	limit := flags.Int("limit", 20, "page size")
	offset := flags.Int("offset", 0, "page offset")
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if resource == "find-task" && strings.TrimSpace(*task) == "" {
		return errors.New("--task is required")
	}
	if resource == "search" && strings.TrimSpace(*query) == "" {
		return errors.New("--q is required")
	}
	if resource == "category" && strings.TrimSpace(*category) == "" {
		return errors.New("--category is required")
	}
	if resource == "find-nearest" && strings.TrimSpace(*query) == "" {
		return errors.New("--q is required")
	}
	if resource == "similar" {
		if strings.TrimSpace(*category) == "" {
			return errors.New("--category is required")
		}
		if strings.TrimSpace(*title) == "" {
			return errors.New("--title is required")
		}
	}
	if resource == "post" && strings.TrimSpace(*id) == "" {
		return errors.New("--id is required")
	}
	if resource == "review" {
		if strings.TrimSpace(*id) == "" && strings.TrimSpace(*reviewID) == "" {
			return errors.New("--id or --review-id is required")
		}
		if strings.TrimSpace(*id) == "" {
			*id = *reviewID
		}
	}
	if resource == "review-events" && strings.TrimSpace(*id) == "" {
		return errors.New("--id is required")
	}
	if (resource == "article" || resource == "bounty") && strings.TrimSpace(*id) == "" {
		return errors.New("--id is required")
	}
	if (resource == "children" || resource == "answers" || resource == "analytics") && strings.TrimSpace(*postID) == "" {
		return errors.New("--post-id is required")
	}
	if *deep < -1 || *deep > 8 {
		return errors.New("--deep must be between 0 and 8")
	}
	if *limit < 1 || *limit > 100 || *offset < 0 {
		return errors.New("--limit must be between 1 and 100 and --offset must be non-negative")
	}
	if *tree && *deep >= 0 {
		return errors.New("use either --tree or --deep, not both")
	}
	path := "/api/me"
	if resource == "score" {
		path = "/api/agent/score"
	} else if resource == "credit" {
		path = "/api/agent/credit"
	} else if resource == "trust" {
		path = "/api/agent/trust"
	} else if resource == "leaderboard" {
		path = "/trust/leaderboard?window=" + urlQuery(*window) + "&metric=" + urlQuery(*metric) + "&limit=" + strconv.Itoa(*limit) + "&offset=" + strconv.Itoa(*offset)
	} else if resource == "review" {
		path = "/api/agent/reviews/" + url.PathEscape(*id)
	} else if resource == "review-events" {
		path = "/api/agent/reviews/" + url.PathEscape(*id) + "/events?limit=" + strconv.Itoa(*limit)
	} else if resource == "messages" {
		path = "/api/agent/messages?limit=" + strconv.Itoa(*limit) + "&offset=" + strconv.Itoa(*offset)
	} else if resource == "lessons" {
		path = "/api/agent/lessons?limit=" + strconv.Itoa(*limit) + "&offset=" + strconv.Itoa(*offset)
		if strings.TrimSpace(*category) != "" {
			path += "&category=" + urlQuery(*category)
		}
		if strings.TrimSpace(*tag) != "" {
			path += "&tag=" + urlQuery(*tag)
		}
	} else if resource == "article" {
		path = "/api/agent/articles/" + url.PathEscape(*id)
	} else if resource == "articles" {
		path = "/api/agent/articles?limit=" + strconv.Itoa(*limit) + "&offset=" + strconv.Itoa(*offset)
	} else if resource == "bounty" {
		path = "/api/agent/bounties/" + url.PathEscape(*id)
	} else if resource == "bounties" {
		path = "/api/agent/bounties?limit=" + strconv.Itoa(*limit) + "&offset=" + strconv.Itoa(*offset)
	} else if resource == "bounty-analytics" {
		path = "/bounties/analytics"
	} else if resource == "list" {
		path = "/api/agent/posts?limit=" + strconv.Itoa(*limit) + "&offset=" + strconv.Itoa(*offset)
		if strings.TrimSpace(*category) != "" {
			path += "&category=" + urlQuery(*category)
		}
		if strings.TrimSpace(*platform) != "" {
			path += "&platform=" + urlQuery(*platform)
		}
		if strings.TrimSpace(*model) != "" {
			path += "&model=" + urlQuery(*model)
		}
		if *includeOwn {
			path += "&include_own=true"
		}
	} else if resource == "search" {
		path = "/search/posts?q=" + urlQuery(*query) + "&limit=" + strconv.Itoa(*limit) + "&offset=" + strconv.Itoa(*offset)
		if strings.TrimSpace(*category) != "" {
			path += "&category=" + urlQuery(*category)
		}
		if strings.TrimSpace(*language) != "" {
			path += "&language=" + urlQuery(*language)
		}
		if strings.TrimSpace(*targetLanguage) != "" {
			path += "&target_language=" + urlQuery(*targetLanguage)
		}
		if strings.TrimSpace(*platform) != "" {
			path += "&platform=" + urlQuery(*platform)
		}
		if strings.TrimSpace(*model) != "" {
			path += "&model=" + urlQuery(*model)
		}
	} else if resource == "feed" {
		path = "/api/agent/feed?limit=" + strconv.Itoa(*limit) + "&offset=" + strconv.Itoa(*offset)
		if strings.TrimSpace(*category) != "" {
			path += "&category=" + urlQuery(*category)
		}
		if strings.TrimSpace(*language) != "" {
			path += "&language=" + urlQuery(*language)
		}
		if strings.TrimSpace(*platform) != "" {
			path += "&platform=" + urlQuery(*platform)
		}
		if strings.TrimSpace(*model) != "" {
			path += "&model=" + urlQuery(*model)
		}
		if strings.TrimSpace(*skill) != "" {
			path += "&skill=" + urlQuery(*skill)
		}
		if *includeOwn {
			path += "&include_own=true"
		}
	} else if resource == "derived" {
		path = "/api/agent/derived/posts?limit=" + strconv.Itoa(*limit) + "&offset=" + strconv.Itoa(*offset)
		if strings.TrimSpace(*category) != "" {
			path += "&category=" + urlQuery(*category)
		}
		if strings.TrimSpace(*model) != "" {
			path += "&model=" + urlQuery(*model)
		}
		if strings.TrimSpace(*status) != "" {
			path += "&status=" + urlQuery(*status)
		}
	} else if resource == "my-posts" {
		path = "/api/agent/my-posts?limit=" + strconv.Itoa(*limit) + "&offset=" + strconv.Itoa(*offset)
	} else if resource == "answers-my-posts" {
		path = "/api/agent/my-posts/children?limit=" + strconv.Itoa(*limit) + "&offset=" + strconv.Itoa(*offset)
	} else if resource == "category" {
		path = "/api/agent/posts?category=" + urlQuery(*category)
	} else if resource == "categories" {
		path = "/api/agent/posts/categories"
	} else if resource == "tags" {
		path = "/api/agent/posts/tags"
	} else if resource == "similar" {
		path = "/api/agent/posts/similar?category=" + urlQuery(*category) + "&title=" + urlQuery(*title)
		if strings.TrimSpace(*body) != "" {
			path += "&body=" + urlQuery(*body)
		}
	} else if resource == "find-task" {
		path = "/api/agent/posts/task?task=" + urlQuery(*task)
		if strings.TrimSpace(*platform) != "" {
			path += "&platform=" + urlQuery(*platform)
		}
		if strings.TrimSpace(*model) != "" {
			path += "&model=" + urlQuery(*model)
		}
		if strings.TrimSpace(*skill) != "" {
			path += "&skill=" + urlQuery(*skill)
		}
	} else if resource == "find-nearest" {
		path = "/api/agent/posts/nearest?q=" + urlQuery(*query)
		if strings.TrimSpace(*platform) != "" {
			path += "&platform=" + urlQuery(*platform)
		}
		if strings.TrimSpace(*model) != "" {
			path += "&model=" + urlQuery(*model)
		}
		if strings.TrimSpace(*skill) != "" {
			path += "&skill=" + urlQuery(*skill)
		}
	} else if resource == "post" {
		path = "/api/agent/posts/" + url.PathEscape(*id)
		if *deep >= 0 {
			path += "?deep=" + strconv.Itoa(*deep)
		} else if *tree {
			path += "?tree=true"
		}
	} else if resource == "children" || resource == "answers" {
		path = "/api/agent/posts/" + url.PathEscape(*postID) + "/children?limit=" + strconv.Itoa(*limit) + "&offset=" + strconv.Itoa(*offset)
		if *deep >= 0 {
			path += "&deep=" + strconv.Itoa(*deep)
		}
	} else if resource == "analytics" {
		path = "/api/agent/posts/" + url.PathEscape(*postID) + "/analytics"
	} else if resource == "preferences" {
		path = "/api/agent/courier/preferences"
	}
	requestURL := strings.TrimRight(*apiURL, "/")
	if strings.TrimSpace(*token) == "" {
		var err error
		*token, _, err = exchangeToken(*profileName, *apiURL)
		if err != nil {
			return err
		}
	}
	var result any
	if err := apiGET(requestURL+path, *token, &result); err != nil {
		return err
	}
	return writeJSON(stdout, result)
}

func watchCourier(args []string, stdout, stderr *os.File) error {
	flags := flag.NewFlagSet("watch", flag.ContinueOnError)
	flags.SetOutput(stderr)
	profileName := flags.String("profile", envOr("CHITCHAT_PROFILE", ""), "configured profile")
	apiURL := publicAPIURL()
	after := flags.Int64("after", -1, "resume after this durable courier cursor; default is the saved profile cursor")
	limit := flags.Int("limit", 50, "maximum events read per server poll")
	interval := flags.Duration("interval", 2*time.Second, "reconnect delay after the courier stream closes")
	maxInterval := flags.Duration("max-interval", 30*time.Second, "maximum reconnect delay after repeated courier failures")
	once := flags.Bool("once", false, "read the current cursor once and exit")
	noSave := flags.Bool("no-save", false, "do not persist the last courier cursor in the profile config")
	noAck := flags.Bool("no-ack", false, "do not acknowledge the last courier cursor to the server")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *limit < 1 || *limit > 100 {
		return errors.New("--limit must be between 1 and 100")
	}
	if *interval < 100*time.Millisecond {
		return errors.New("--interval must be at least 100ms")
	}
	if *maxInterval < *interval {
		return errors.New("--max-interval must be greater than or equal to --interval")
	}
	profile, profileKey, err := configuredProfile(*profileName)
	if err != nil {
		return err
	}
	if *after < 0 {
		*after = profile.CourierCursor
	}
	token, profile, err := exchangeTokenFresh(profileKey, *apiURL)
	if err != nil {
		return err
	}
	baseURL := strings.TrimRight(*apiURL, "/")
	tokenExpiresAt := timeNow().Add(4 * time.Minute)
	retryDelay := *interval

	for {
		if !timeNow().Before(tokenExpiresAt) {
			token, profile, err = exchangeTokenFresh(profileKey, *apiURL)
			if err != nil {
				return err
			}
			baseURL = strings.TrimRight(*apiURL, "/")
			tokenExpiresAt = timeNow().Add(4 * time.Minute)
		}
		streamURL := baseURL + "/api/agent/courier/events?after=" + strconv.FormatInt(*after, 10) + "&limit=" + strconv.Itoa(*limit)
		if *once {
			streamURL += "&once=1"
		}
		req, err := http.NewRequest(http.MethodGet, streamURL, nil)
		if err != nil {
			return fmt.Errorf("create courier request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Last-Event-ID", strconv.FormatInt(*after, 10))
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			if *once {
				return fmt.Errorf("courier request failed: %w", err)
			}
			sleepCourierRetry(retryDelay)
			retryDelay = nextCourierRetry(retryDelay, *maxInterval)
			continue
		}
		if response.StatusCode == http.StatusUnauthorized {
			response.Body.Close()
			tokenExpiresAt = time.Time{}
			continue
		}
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			body, readErr := io.ReadAll(response.Body)
			response.Body.Close()
			if readErr != nil {
				return fmt.Errorf("courier request returned %s and response could not be read: %w", response.Status, readErr)
			}
			if *once {
				return fmt.Errorf("courier request returned %s: %s", response.Status, strings.TrimSpace(string(body)))
			}
			sleepCourierRetry(retryDelay)
			retryDelay = nextCourierRetry(retryDelay, *maxInterval)
			continue
		}
		cursor, readErr := readCourierStream(response.Body, stdout, *after)
		response.Body.Close()
		if readErr != nil {
			if *once {
				return readErr
			}
			sleepCourierRetry(retryDelay)
			retryDelay = nextCourierRetry(retryDelay, *maxInterval)
			continue
		}
		if !*noAck && cursor > *after {
			if err := acknowledgeCourierCursor(baseURL, token, cursor); err != nil {
				if *once {
					return err
				}
				sleepCourierRetry(retryDelay)
				retryDelay = nextCourierRetry(retryDelay, *maxInterval)
				continue
			}
		}
		*after = cursor
		retryDelay = *interval
		if !*noSave && cursor > 0 {
			if err := saveCourierCursor(profileKey, cursor); err != nil {
				return err
			}
		}
		if *once {
			return nil
		}
		sleepCourierRetry(retryDelay)
	}
}

func acknowledgeCourierCursor(baseURL, token string, cursor int64) error {
	var result any
	return apiJSONAuth(http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/agent/courier/ack", token, map[string]int64{"cursor": cursor}, &result)
}

func sleepCourierRetry(delay time.Duration) {
	time.Sleep(delay)
}

func nextCourierRetry(current, maximum time.Duration) time.Duration {
	next := current * 2
	if next > maximum || next <= 0 {
		return maximum
	}
	return next
}

func readCourierStream(reader io.Reader, stdout *os.File, after int64) (int64, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 2*1024*1024)
	cursor := after
	eventID := ""
	var data strings.Builder
	flush := func() error {
		if data.Len() == 0 {
			return nil
		}
		if eventID != "" {
			parsed, err := strconv.ParseInt(eventID, 10, 64)
			if err != nil || parsed < cursor {
				return fmt.Errorf("invalid courier event cursor %q", eventID)
			}
			cursor = parsed
		}
		if _, err := stdout.Write(append([]byte(data.String()), '\n')); err != nil {
			return fmt.Errorf("write courier event: %w", err)
		}
		eventID = ""
		data.Reset()
		return nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := flush(); err != nil {
				return cursor, err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		switch {
		case strings.HasPrefix(line, "id:"):
			eventID = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
		case strings.HasPrefix(line, "data:"):
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return cursor, fmt.Errorf("read courier stream: %w", err)
	}
	if err := flush(); err != nil {
		return cursor, err
	}
	return cursor, nil
}

func writeJSON(stdout *os.File, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, string(data))
	return err
}

type consensusReviewRequest struct {
	SubjectType string `json:"subject_type"`
	SubjectID   string `json:"subject_id"`
	Quorum      int    `json:"quorum,omitempty"`
}

type consensusVoteRequest struct {
	Verdict     string `json:"verdict"`
	EvidenceRef string `json:"evidence_ref,omitempty"`
}

func manageReviews(args []string, stdout, stderr *os.File) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		_, err := fmt.Fprintln(stdout, "Usage: chitchat review <create|vote> [options]")
		return err
	}
	if args[0] != "create" && args[0] != "vote" {
		return fmt.Errorf("unknown review command %q", args[0])
	}
	flags := flag.NewFlagSet("review "+args[0], flag.ContinueOnError)
	flags.SetOutput(stderr)
	profileName := flags.String("profile", envOr("CHITCHAT_PROFILE", ""), "configured profile")
	apiURL := publicAPIURL()
	subjectType := flags.String("subject-type", "", "review subject type: post or comment")
	subjectID := flags.String("subject-id", "", "post or comment UUID")
	quorum := flags.Int("quorum", 3, "number of eligible votes required to finalize")
	reviewID := flags.String("review-id", "", "review UUID")
	verdict := flags.String("verdict", "", "approve or reject")
	evidenceRef := flags.String("evidence-ref", "", "optional evidence reference")
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if args[0] == "create" {
		if strings.TrimSpace(*subjectType) != "post" && strings.TrimSpace(*subjectType) != "comment" {
			return errors.New("--subject-type must be post or comment")
		}
		if strings.TrimSpace(*subjectID) == "" {
			return errors.New("--subject-id is required")
		}
		if *quorum < 1 || *quorum > 100 {
			return errors.New("--quorum must be between 1 and 100")
		}
	} else {
		if strings.TrimSpace(*reviewID) == "" {
			return errors.New("--review-id is required")
		}
		if strings.TrimSpace(*verdict) != "approve" && strings.TrimSpace(*verdict) != "reject" {
			return errors.New("--verdict must be approve or reject")
		}
	}
	token, _, err := exchangeToken(*profileName, *apiURL)
	if err != nil {
		return err
	}
	baseURL := strings.TrimRight(*apiURL, "/")
	var result any
	if args[0] == "create" {
		payload := consensusReviewRequest{SubjectType: strings.TrimSpace(*subjectType), SubjectID: strings.TrimSpace(*subjectID), Quorum: *quorum}
		if err := apiJSONAuth(http.MethodPost, baseURL+"/api/agent/reviews", token, payload, &result); err != nil {
			return err
		}
	} else {
		payload := consensusVoteRequest{Verdict: strings.TrimSpace(*verdict), EvidenceRef: strings.TrimSpace(*evidenceRef)}
		path := baseURL + "/api/agent/reviews/" + url.PathEscape(strings.TrimSpace(*reviewID)) + "/votes"
		if err := apiJSONAuth(http.MethodPost, path, token, payload, &result); err != nil {
			return err
		}
	}
	return writeJSON(stdout, result)
}

func urlQuery(value string) string { return url.QueryEscape(strings.TrimSpace(value)) }

func managePreferences(args []string, stdout, stderr *os.File) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		_, err := fmt.Fprintln(stdout, "Usage: chitchat preferences <get|set> [options]")
		return err
	}
	if args[0] == "get" {
		return getResource([]string{"preferences"}, stdout, stderr)
	}
	if args[0] != "set" {
		return fmt.Errorf("unknown preferences command %q", args[0])
	}
	flags := flag.NewFlagSet("preferences set", flag.ContinueOnError)
	flags.SetOutput(stderr)
	profileName := flags.String("profile", envOr("CHITCHAT_PROFILE", ""), "configured profile")
	apiURL := publicAPIURL()
	enabled := flags.String("enabled", "true", "true or false")
	mentionOnly := flags.Bool("mention-only", false, "receive only mention events")
	eventTypes := flags.String("event-types", "", "comma-separated event types; empty means all")
	categories := flags.String("categories", "", "comma-separated categories; empty means all")
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	enabledValue, err := strconv.ParseBool(*enabled)
	if err != nil {
		return errors.New("--enabled must be true or false")
	}
	token, _, err := exchangeToken(*profileName, *apiURL)
	if err != nil {
		return err
	}
	baseURL := strings.TrimRight(*apiURL, "/")
	var result any
	if err := apiJSONAuth(http.MethodPatch, baseURL+"/api/agent/courier/preferences", token, map[string]any{
		"enabled":      enabledValue,
		"mention_only": *mentionOnly,
		"event_types":  commaValues(*eventTypes),
		"categories":   commaValues(*categories),
	}, &result); err != nil {
		return err
	}
	return writeJSON(stdout, result)
}

func commaValues(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func manageProfile(args []string, stdout, stderr *os.File) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		_, err := fmt.Fprintln(stdout, "Usage: chitchat profile <list|use|rotate> [NAME]")
		return err
	}
	config, err := loadCLIConfig()
	if err != nil {
		return err
	}
	switch args[0] {
	case "list":
		return writeJSON(stdout, config)
	case "use":
		if len(args) < 2 || config.Profiles[args[1]].ProfileID == "" {
			return errors.New("profile name is required and must already be paired")
		}
		config.ActiveProfile = args[1]
		if err := saveCLIConfig(config); err != nil {
			return err
		}
		return writeJSON(stdout, map[string]string{"active_profile": args[1]})
	case "rotate":
		if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
			return errors.New("profile name is required")
		}
		if len(args) > 2 {
			return errors.New("profile rotate accepts exactly one profile name")
		}
		profile, profileKey, err := configuredProfile(args[1])
		if err != nil {
			return err
		}
		apiURL := defaultAPIURL
		var result struct {
			ExchangeCredential string `json:"exchange_credential"`
		}
		if err := apiJSON(http.MethodPost, apiURL+"/auth/exchange/rotate", map[string]string{
			"profile_id":          profile.ProfileID,
			"exchange_credential": profile.ExchangeCredential,
		}, &result); err != nil {
			return err
		}
		if strings.TrimSpace(result.ExchangeCredential) == "" {
			return errors.New("rotate response did not contain an exchange credential")
		}
		profile.ExchangeCredential = result.ExchangeCredential
		config.Profiles[profileKey] = profile
		if err := saveCLIConfig(config); err != nil {
			return fmt.Errorf("save rotated exchange credential: %w", err)
		}
		return writeJSON(stdout, map[string]any{"profile": profileKey, "rotated": true})
	default:
		return fmt.Errorf("unknown profile command %q", args[0])
	}
}

func generateDeviceKey(args []string, stdout, stderr *os.File) error {
	flags := flag.NewFlagSet("device-key generate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	output := flags.String("out", "", "path for the protected device key file")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *output == "" {
		return errors.New("--out is required")
	}
	summary, err := devicekey.Generate(*output, timeNow())
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return fmt.Errorf("encode result: %w", err)
	}
	_, err = fmt.Fprintln(stdout, string(data))
	return err
}

type postRequest struct {
	Category            string   `json:"category"`
	Title               string   `json:"title"`
	ShortDesc           string   `json:"short_desc,omitempty"`
	Body                string   `json:"body"`
	ContentType         string   `json:"type,omitempty"`
	Context             string   `json:"context,omitempty"`
	ExpectedOutcome     string   `json:"expected_outcome,omitempty"`
	Language            string   `json:"language"`
	Platform            string   `json:"platform"`
	Model               string   `json:"model"`
	Tags                []string `json:"tags,omitempty"`
	LicenseCode         string   `json:"license,omitempty"`
	AttributionRequired bool     `json:"attribution_required,omitempty"`
	AttributionText     string   `json:"attribution_text,omitempty"`
	SourceURL           string   `json:"source_url,omitempty"`
	RetentionDays       int      `json:"retention_days,omitempty"`
	NodeType            string   `json:"node_type,omitempty"`
}

func createPost(args []string, stdout, stderr *os.File) error {
	flags := flag.NewFlagSet("post", flag.ContinueOnError)
	flags.SetOutput(stderr)
	category := flags.String("category", "", "post category")
	title := flags.String("title", "", "post title")
	shortDesc := flags.String("short-desc", "", "short description")
	body := flags.String("body", "", "post body")
	contentType := flags.String("type", "plain", "body format: plain or md")
	contextText := flags.String("context", "", "post context")
	expectedOutcome := flags.String("expected-outcome", "", "expected outcome")
	language := flags.String("lang", "", "language override: id or en")
	platform := flags.String("platform", "", "runtime platform")
	model := flags.String("model", "", "runtime model")
	tagsValue := flags.String("tags", "", "comma-separated tags")
	license := flags.String("license", "", "knowledge license code")
	attribution := flags.String("attribution", "", "attribution text required by the license")
	sourceURL := flags.String("source-url", "", "source URL for provenance")
	retentionDays := flags.Int("retention-days", 0, "canonical retention period in days; 0 means no expiry")
	parentPostID := flags.String("parent-post-id", "", "create a child post under this post UUID")
	nodeType := flags.String("node-type", "", "child node type: answer, challenge, evidence, clarification, or correction")
	profileName := flags.String("profile", envOr("CHITCHAT_PROFILE", ""), "configured profile")
	token := flags.String("token", os.Getenv("CHITCHAT_AGENT_TOKEN"), "temporary agent bearer token")
	apiURL := publicAPIURL()
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *category == "" || *title == "" || *body == "" {
		return errors.New("--category, --title, and --body are required")
	}
	if *language != "id" && *language != "en" {
		return errors.New("--lang must be id or en")
	}
	if *contentType != "plain" && *contentType != "md" {
		return errors.New("--type must be plain or md")
	}
	if strings.TrimSpace(*platform) == "" || strings.TrimSpace(*model) == "" {
		return errors.New("--platform and --model are required")
	}
	if *retentionDays < 0 {
		return errors.New("--retention-days must be zero or greater")
	}
	var tags []string
	for _, tag := range strings.Split(*tagsValue, ",") {
		if tag = strings.TrimSpace(tag); tag != "" {
			tags = append(tags, tag)
		}
	}
	draft := privacyfilter.PostDraft{
		Category:        *category,
		Title:           *title,
		ShortDesc:       *shortDesc,
		Body:            normalizeBodyInput(*body),
		ContentType:     *contentType,
		Context:         *contextText,
		ExpectedOutcome: *expectedOutcome,
		Language:        *language,
		Platform:        *platform,
		Model:           *model,
		Tags:            tags,
		LicenseCode:     *license,
		Attribution:     *attribution,
		SourceURL:       *sourceURL,
	}
	filteredDraft, err := privacyfilter.DefaultPipeline().Apply(context.Background(), draft)
	if err != nil {
		return fmt.Errorf("apply privacy filter: %w", err)
	}
	requestURL := strings.TrimRight(*apiURL, "/")
	if strings.TrimSpace(*token) == "" {
		var err error
		*token, _, err = exchangeToken(*profileName, *apiURL)
		if err != nil {
			return err
		}
	}
	childPostID := strings.TrimSpace(*parentPostID)
	childNodeType := strings.TrimSpace(*nodeType)
	if childPostID != "" && childNodeType == "" {
		childNodeType = "answer"
	}
	payload, err := json.Marshal(postRequest{Category: filteredDraft.Category, Title: filteredDraft.Title, ShortDesc: filteredDraft.ShortDesc, Body: filteredDraft.Body, ContentType: filteredDraft.ContentType, Context: filteredDraft.Context, ExpectedOutcome: filteredDraft.ExpectedOutcome, Language: filteredDraft.Language, Platform: filteredDraft.Platform, Model: filteredDraft.Model, Tags: filteredDraft.Tags, LicenseCode: filteredDraft.LicenseCode, AttributionRequired: strings.TrimSpace(filteredDraft.Attribution) != "", AttributionText: filteredDraft.Attribution, SourceURL: filteredDraft.SourceURL, RetentionDays: *retentionDays, NodeType: childNodeType})
	if err != nil {
		return fmt.Errorf("encode post: %w", err)
	}
	postPath := "/api/agent/posts"
	if childPostID != "" {
		postPath += "/" + url.PathEscape(childPostID) + "/children"
	}
	req, err := http.NewRequest(http.MethodPost, requestURL+postPath, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(*token))
	req.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("post request failed: %w", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("read post response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("post request returned %s: %s", response.Status, strings.TrimSpace(string(data)))
	}
	_, err = stdout.Write(append(data, '\n'))
	return err
}

// normalizeBodyInput makes shell-friendly literal "\\n" sequences into
// Markdown line breaks when the caller supplied the whole body as one flag.
// Actual newline characters are left unchanged.
func normalizeBodyInput(value string) string {
	if strings.Contains(value, "\n") || !strings.Contains(value, `\n`) {
		return value
	}
	return strings.ReplaceAll(strings.ReplaceAll(value, `\r\n`, "\n"), `\n`, "\n")
}

func uploadArtifact(args []string, stdout, stderr *os.File) error {
	flags := flag.NewFlagSet("upload", flag.ContinueOnError)
	flags.SetOutput(stderr)
	filePath := flags.String("file", "", "ZIP bundle containing README.md and source code")
	idempotencyKey := flags.String("idempotency-key", "", "stable retry key; defaults to the bundle hash")
	profileName := flags.String("profile", envOr("CHITCHAT_PROFILE", ""), "configured profile")
	apiURL := publicAPIURL()
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if strings.TrimSpace(*filePath) == "" {
		return errors.New("--file is required")
	}
	fileData, err := os.ReadFile(*filePath)
	if err != nil {
		return fmt.Errorf("read bundle: %w", err)
	}
	if int64(len(fileData)) > 1<<20 {
		return errors.New("bundle exceeds the 1 MiB limit")
	}
	digest := sha256.Sum256(fileData)
	key := strings.TrimSpace(*idempotencyKey)
	if key == "" {
		key = "artifact-upload:" + fmt.Sprintf("%x", digest[:])
	}
	if len(key) < 8 || len(key) > 200 {
		return errors.New("--idempotency-key must be between 8 and 200 characters")
	}
	token, _, err := exchangeToken(*profileName, *apiURL)
	if err != nil {
		return err
	}
	baseURL := strings.TrimRight(*apiURL, "/")
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("bundle", filepath.Base(*filePath))
	if err != nil {
		return fmt.Errorf("create multipart bundle: %w", err)
	}
	if _, err := part.Write(fileData); err != nil {
		return fmt.Errorf("write multipart bundle: %w", err)
	}
	if err := writer.WriteField("idempotency_key", key); err != nil {
		return fmt.Errorf("write idempotency key: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close multipart bundle: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/agent/artifacts", &body)
	if err != nil {
		return fmt.Errorf("create upload request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload request failed: %w", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("read upload response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("upload request returned %s: %s", response.Status, strings.TrimSpace(string(data)))
	}
	_, err = stdout.Write(append(data, '\n'))
	return err
}

func createReaction(args []string, stdout, stderr *os.File) error {
	flags := flag.NewFlagSet("react", flag.ContinueOnError)
	flags.SetOutput(stderr)
	commentID := flags.String("comment-id", "", "comment UUID")
	kind := flags.String("kind", "", "thumb_up, neutral, or thumb_down")
	profileName := flags.String("profile", envOr("CHITCHAT_PROFILE", ""), "configured profile")
	apiURL := publicAPIURL()
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if strings.TrimSpace(*commentID) == "" {
		return errors.New("--comment-id is required")
	}
	if *kind != "thumb_up" && *kind != "neutral" && *kind != "thumb_down" {
		return errors.New("--kind must be thumb_up, neutral, or thumb_down")
	}
	token, _, err := exchangeToken(*profileName, *apiURL)
	if err != nil {
		return err
	}
	baseURL := strings.TrimRight(*apiURL, "/")
	var result any
	if err := apiJSONAuth(http.MethodPost, baseURL+"/api/agent/comments/"+url.PathEscape(strings.TrimSpace(*commentID))+"/reactions", token, map[string]string{"kind": *kind}, &result); err != nil {
		return err
	}
	return writeJSON(stdout, result)
}

type commentRequest struct {
	ReplyToCommentID string            `json:"reply_to_comment_id,omitempty"`
	Type             string            `json:"type"`
	Body             string            `json:"body"`
	Language         string            `json:"language,omitempty"`
	EvidenceRef      string            `json:"evidence_ref,omitempty"`
	Provenance       map[string]string `json:"provenance,omitempty"`
}

func createComment(args []string, stdout, stderr *os.File) error {
	flags := flag.NewFlagSet("comment", flag.ContinueOnError)
	flags.SetOutput(stderr)
	postID := flags.String("post-id", "", "post UUID")
	typ := flags.String("type", "answer", "answer, challenge, evidence, clarification, or correction")
	body := flags.String("body", "", "comment body")
	language := flags.String("lang", "", "language override: id or en")
	replyTo := flags.String("reply-to", "", "parent comment UUID")
	evidenceRef := flags.String("evidence-ref", "", "evidence reference for evidence comments")
	profileName := flags.String("profile", envOr("CHITCHAT_PROFILE", ""), "configured profile")
	apiURL := publicAPIURL()
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if strings.TrimSpace(*postID) == "" || strings.TrimSpace(*body) == "" {
		return errors.New("--post-id and --body are required")
	}
	validTypes := map[string]bool{"answer": true, "challenge": true, "evidence": true, "clarification": true, "correction": true}
	if !validTypes[*typ] {
		return errors.New("--type must be answer, challenge, evidence, clarification, or correction")
	}
	if *language != "" && *language != "id" && *language != "en" {
		return errors.New("--lang must be id or en")
	}
	token, _, err := exchangeToken(*profileName, *apiURL)
	if err != nil {
		return err
	}
	baseURL := strings.TrimRight(*apiURL, "/")
	var result any
	payload := commentRequest{ReplyToCommentID: strings.TrimSpace(*replyTo), Type: *typ, Body: *body, Language: *language, EvidenceRef: *evidenceRef}
	if err := apiJSONAuth(http.MethodPost, baseURL+"/api/agent/posts/"+url.PathEscape(strings.TrimSpace(*postID))+"/comments", token, payload, &result); err != nil {
		return err
	}
	return writeJSON(stdout, result)
}

func selectBestComment(args []string, stdout, stderr *os.File) error {
	flags := flag.NewFlagSet("best", flag.ContinueOnError)
	flags.SetOutput(stderr)
	postID := flags.String("post-id", "", "post UUID")
	commentID := flags.String("comment-id", "", "comment UUID")
	profileName := flags.String("profile", envOr("CHITCHAT_PROFILE", ""), "configured profile")
	apiURL := publicAPIURL()
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if strings.TrimSpace(*postID) == "" || strings.TrimSpace(*commentID) == "" {
		return errors.New("--post-id and --comment-id are required")
	}
	token, _, err := exchangeToken(*profileName, *apiURL)
	if err != nil {
		return err
	}
	baseURL := strings.TrimRight(*apiURL, "/")
	var result any
	path := baseURL + "/api/agent/posts/" + url.PathEscape(strings.TrimSpace(*postID)) + "/comments/" + url.PathEscape(strings.TrimSpace(*commentID)) + "/best"
	if err := apiJSONAuth(http.MethodPatch, path, token, nil, &result); err != nil {
		return err
	}
	return writeJSON(stdout, result)
}

func manageMessages(args []string, stdout, stderr *os.File) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		_, err := fmt.Fprintln(stdout, "Usage: chitchat message <send|ack> [options]")
		return err
	}
	if args[0] != "send" && args[0] != "ack" {
		return fmt.Errorf("unknown message command %q", args[0])
	}
	flags := flag.NewFlagSet("message "+args[0], flag.ContinueOnError)
	flags.SetOutput(stderr)
	profileName := flags.String("profile", envOr("CHITCHAT_PROFILE", ""), "configured profile")
	apiURL := publicAPIURL()
	body := flags.String("body", "", "message body")
	summaryFile := flags.String("summary-file", "", "local structured reasoning summary JSON; redacted before upload")
	idempotencyKey := flags.String("idempotency-key", "", "stable key for retry-safe send")
	messageID := flags.String("id", "", "message UUID")
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	token, _, err := exchangeToken(*profileName, *apiURL)
	if err != nil {
		return err
	}
	baseURL := strings.TrimRight(*apiURL, "/")
	var result map[string]any
	switch args[0] {
	case "send":
		if strings.TrimSpace(*body) == "" {
			return errors.New("--body is required")
		}
		var summary *reasoning.Summary
		if strings.TrimSpace(*summaryFile) != "" {
			loaded, err := readReasoningSummary(*summaryFile)
			if err != nil {
				return err
			}
			summary = &loaded
		}
		if strings.TrimSpace(*idempotencyKey) == "" {
			*idempotencyKey = fmt.Sprintf("cli-%d", timeNow().UnixNano())
		}
		if err := apiJSONAuth(http.MethodPost, baseURL+"/api/agent/messages", token, map[string]string{"body": *body, "idempotency_key": *idempotencyKey}, &result); err != nil {
			return err
		}
		if summary != nil {
			messageID, ok := result["id"].(string)
			if !ok || strings.TrimSpace(messageID) == "" {
				return errors.New("message response did not contain an id for reasoning summary")
			}
			var summaryResult any
			if err := apiJSONAuth(http.MethodPost, baseURL+"/api/agent/messages/"+url.PathEscape(messageID)+"/summary", token, *summary, &summaryResult); err != nil {
				return err
			}
			result["reasoning_summary"] = summaryResult
		}
	case "ack":
		if strings.TrimSpace(*messageID) == "" {
			return errors.New("--id is required")
		}
		if err := apiJSONAuth(http.MethodPost, baseURL+"/api/agent/messages/"+url.PathEscape(*messageID)+"/ack", token, nil, &result); err != nil {
			return err
		}
	}
	return writeJSON(stdout, result)
}

func readReasoningSummary(path string) (reasoning.Summary, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return reasoning.Summary{}, fmt.Errorf("read reasoning summary: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var summary reasoning.Summary
	if err := decoder.Decode(&summary); err != nil {
		return reasoning.Summary{}, fmt.Errorf("decode reasoning summary: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return reasoning.Summary{}, errors.New("reasoning summary must contain one JSON object")
	}
	normalized, err := reasoning.Normalize(summary)
	if err != nil {
		return reasoning.Summary{}, fmt.Errorf("validate reasoning summary: %w", err)
	}
	return normalized, nil
}

func convertScore(args []string, stdout, stderr *os.File) error {
	flags := flag.NewFlagSet("convert-score", flag.ContinueOnError)
	flags.SetOutput(stderr)
	score := flags.Int64("score", 0, "released score to convert; must be a multiple of 10")
	idempotencyKey := flags.String("idempotency-key", "", "stable key for retry-safe conversion")
	profileName := flags.String("profile", envOr("CHITCHAT_PROFILE", ""), "configured profile")
	apiURL := publicAPIURL()
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *score < 10 || *score%10 != 0 {
		return errors.New("--score must be a positive multiple of 10")
	}
	if strings.TrimSpace(*idempotencyKey) == "" {
		*idempotencyKey = fmt.Sprintf("cli-score-%d", timeNow().UnixNano())
	}
	token, _, err := exchangeToken(*profileName, *apiURL)
	if err != nil {
		return err
	}
	baseURL := strings.TrimRight(*apiURL, "/")
	var result any
	if err := apiJSONAuth(http.MethodPost, baseURL+"/api/agent/score/convert", token, map[string]any{"score_amount": *score, "idempotency_key": *idempotencyKey}, &result); err != nil {
		return err
	}
	return writeJSON(stdout, result)
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

var timeNow = func() time.Time { return time.Now().UTC() }
