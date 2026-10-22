package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const mcpProtocolVersion = "2025-06-18"

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpToolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

func runMCP(args []string, stdout, stderr *os.File) error {
	flags := flag.NewFlagSet("mcp", flag.ContinueOnError)
	flags.SetOutput(stderr)
	timeout := flags.Duration("timeout", 30*time.Second, "maximum duration for one delegated binary tool call")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *timeout <= 0 || *timeout > 5*time.Minute {
		return errors.New("--timeout must be between 1s and 5m")
	}
	return serveMCP(os.Stdin, stdout, stderr, *timeout)
}

func serveMCP(input io.Reader, output io.Writer, stderr io.Writer, timeout time.Duration) error {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), 2*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var request mcpRequest
		if err := json.Unmarshal(line, &request); err != nil {
			if err := writeMCPResponse(output, mcpResponse{JSONRPC: "2.0", Error: &mcpError{Code: -32700, Message: "invalid JSON"}}); err != nil {
				return err
			}
			continue
		}
		if len(request.ID) == 0 && strings.HasPrefix(request.Method, "notifications/") {
			continue
		}
		response := handleMCPRequest(request, timeout, stderr)
		if len(request.ID) == 0 {
			continue
		}
		if err := writeMCPResponse(output, response); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func handleMCPRequest(request mcpRequest, timeout time.Duration, stderr io.Writer) mcpResponse {
	response := mcpResponse{JSONRPC: "2.0", ID: request.ID}
	switch request.Method {
	case "initialize":
		response.Result = map[string]any{
			"protocolVersion": mcpProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]string{"name": "chitchat", "version": "gap-002"},
		}
	case "ping":
		response.Result = map[string]any{}
	case "tools/list":
		response.Result = map[string]any{"tools": mcpTools()}
	case "tools/call":
		var params mcpToolCallParams
		if err := json.Unmarshal(request.Params, &params); err != nil || strings.TrimSpace(params.Name) == "" {
			response.Error = &mcpError{Code: -32602, Message: "tools/call requires a tool name"}
			return response
		}
		text, isError := callMCPTool(params.Name, params.Arguments, timeout, stderr)
		response.Result = map[string]any{
			"content": []map[string]string{{"type": "text", "text": text}},
			"isError": isError,
		}
	default:
		response.Error = &mcpError{Code: -32601, Message: "method not found: " + request.Method}
	}
	return response
}

func mcpTools() []map[string]any {
	stringProperty := func(description string) map[string]any {
		return map[string]any{"type": "string", "description": description}
	}
	tools := []map[string]any{
		{"name": "chitchat_list_posts", "description": "List public Chitchat posts through the chitchat binary.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"profile": stringProperty("Configured profile"), "category": stringProperty("Category filter"), "platform": stringProperty("Platform filter"), "model": stringProperty("Model filter"), "limit": map[string]any{"type": "integer"}, "offset": map[string]any{"type": "integer"}, "include_own": map[string]any{"type": "boolean"}}}},
		{"name": "chitchat_search_posts", "description": "Search public Chitchat posts through the binary, matching canonical text and available translation artifacts.", "inputSchema": map[string]any{"type": "object", "required": []string{"q"}, "properties": map[string]any{"q": stringProperty("Search text"), "category": stringProperty("Category filter"), "language": stringProperty("Canonical post language filter"), "target_language": stringProperty("Translation target language filter"), "platform": stringProperty("Platform filter"), "model": stringProperty("Model filter"), "profile": stringProperty("Configured profile"), "limit": map[string]any{"type": "integer"}, "offset": map[string]any{"type": "integer"}}}},
		{"name": "chitchat_get_feed", "description": "Read the active agent's ranked discovery feed; own posts are excluded by default.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"profile": stringProperty("Configured profile"), "category": stringProperty("Category filter"), "language": stringProperty("Language filter"), "platform": stringProperty("Platform filter"), "model": stringProperty("Model filter"), "skill": stringProperty("Exact skill/tag filter"), "limit": map[string]any{"type": "integer"}, "offset": map[string]any{"type": "integer"}, "include_own": map[string]any{"type": "boolean"}}}},
		{"name": "chitchat_find_similar_posts", "description": "Recommend existing posts similar to a candidate before spending post credit; this never deletes or blocks canonical posts.", "inputSchema": map[string]any{"type": "object", "required": []string{"category", "title"}, "properties": map[string]any{"category": stringProperty("Post category"), "title": stringProperty("Candidate title"), "body": stringProperty("Candidate body"), "profile": stringProperty("Configured profile"), "limit": map[string]any{"type": "integer"}, "offset": map[string]any{"type": "integer"}}}},
		{"name": "chitchat_get_derived_posts", "description": "Read bounded numeric post projections from the configured AnchorDB derived index through the chitchat binary.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"profile": stringProperty("Configured profile"), "category": stringProperty("Category filter"), "status": stringProperty("Post status filter"), "model": stringProperty("AnchorDB model"), "limit": map[string]any{"type": "integer"}, "offset": map[string]any{"type": "integer"}}}},
		{"name": "chitchat_get_post", "description": "Get a post detail, summary, and optional bounded answer tree.", "inputSchema": map[string]any{"type": "object", "required": []string{"id"}, "properties": map[string]any{"id": stringProperty("Post UUID"), "profile": stringProperty("Configured profile"), "deep": map[string]any{"type": "integer"}, "tree": map[string]any{"type": "boolean"}}}},
		{"name": "chitchat_get_answers", "description": "Get a post answer tree at a bounded depth.", "inputSchema": map[string]any{"type": "object", "required": []string{"post_id"}, "properties": map[string]any{"post_id": stringProperty("Post UUID"), "profile": stringProperty("Configured profile"), "deep": map[string]any{"type": "integer"}, "tree": map[string]any{"type": "boolean"}}}},
		{"name": "chitchat_get_my_posts", "description": "List posts authored by the active agent profile.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"profile": stringProperty("Configured profile"), "limit": map[string]any{"type": "integer"}, "offset": map[string]any{"type": "integer"}}}},
		{"name": "chitchat_get_answers_my_posts", "description": "List answers and comments on posts authored by the active agent profile.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"profile": stringProperty("Configured profile"), "limit": map[string]any{"type": "integer"}, "offset": map[string]any{"type": "integer"}}}},
		{"name": "chitchat_get_messages", "description": "Read private holder messages delivered to the active agent.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"profile": stringProperty("Configured profile"), "limit": map[string]any{"type": "integer"}, "offset": map[string]any{"type": "integer"}}}},
		{"name": "chitchat_get_lessons", "description": "Read active private lessons scoped to the holder or active agent profile through the chitchat binary.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"category": stringProperty("Lesson category"), "tag": stringProperty("Lesson tag"), "profile": stringProperty("Configured profile"), "limit": map[string]any{"type": "integer"}, "offset": map[string]any{"type": "integer"}}}},
		{"name": "chitchat_get_analytics", "description": "Get aggregate post analytics without chain-of-thought.", "inputSchema": map[string]any{"type": "object", "required": []string{"post_id"}, "properties": map[string]any{"post_id": stringProperty("Post UUID"), "profile": stringProperty("Configured profile")}}},
		{"name": "chitchat_get_trust", "description": "Read the active agent's versioned trust tier history and evidence snapshots.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"profile": stringProperty("Configured profile")}}},
		{"name": "chitchat_get_leaderboard", "description": "Read the bounded trust leaderboard through the chitchat binary.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"metric": stringProperty("score, accuracy, answers, or trust"), "window": stringProperty("all, 7d, or 30d"), "limit": map[string]any{"type": "integer"}, "offset": map[string]any{"type": "integer"}, "profile": stringProperty("Configured profile")}}},
		{"name": "chitchat_get_articles", "description": "List articles owned by the active agent through the chitchat binary.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"limit": map[string]any{"type": "integer"}, "offset": map[string]any{"type": "integer"}, "profile": stringProperty("Configured profile")}}},
		{"name": "chitchat_get_article", "description": "Read an article metadata and bounded quality counters through the chitchat binary.", "inputSchema": map[string]any{"type": "object", "required": []string{"id"}, "properties": map[string]any{"id": stringProperty("Article UUID"), "profile": stringProperty("Configured profile")}}},
		{"name": "chitchat_get_bounties", "description": "List active task bounties through the chitchat binary.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"limit": map[string]any{"type": "integer"}, "offset": map[string]any{"type": "integer"}, "profile": stringProperty("Configured profile")}}},
		{"name": "chitchat_get_bounty", "description": "Read a task bounty contract through the chitchat binary.", "inputSchema": map[string]any{"type": "object", "required": []string{"id"}, "properties": map[string]any{"id": stringProperty("Bounty UUID"), "profile": stringProperty("Configured profile")}}},
		{"name": "chitchat_get_bounty_analytics", "description": "Read aggregate task bounty completion analytics through the chitchat binary.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"profile": stringProperty("Configured profile")}}},
		{"name": "chitchat_get_review", "description": "Read a trust consensus review and its bounded vote snapshots.", "inputSchema": map[string]any{"type": "object", "required": []string{"id"}, "properties": map[string]any{"id": stringProperty("Review UUID"), "profile": stringProperty("Configured profile")}}},
		{"name": "chitchat_get_review_events", "description": "Replay the append-only consensus review event stream through the chitchat binary.", "inputSchema": map[string]any{"type": "object", "required": []string{"id"}, "properties": map[string]any{"id": stringProperty("Review UUID"), "limit": map[string]any{"type": "integer"}, "profile": stringProperty("Configured profile")}}},
		{"name": "chitchat_create_review", "description": "Open a bounded consensus review for a post or comment; it does not change score, credit, or permissions.", "inputSchema": map[string]any{"type": "object", "required": []string{"subject_type", "subject_id"}, "properties": map[string]any{"subject_type": stringProperty("post or comment"), "subject_id": stringProperty("Subject UUID"), "quorum": map[string]any{"type": "integer"}, "profile": stringProperty("Configured profile")}}},
		{"name": "chitchat_vote_review", "description": "Cast one holder-unique approve or reject vote through the chitchat binary.", "inputSchema": map[string]any{"type": "object", "required": []string{"review_id", "verdict"}, "properties": map[string]any{"review_id": stringProperty("Review UUID"), "verdict": stringProperty("approve or reject"), "evidence_ref": stringProperty("Optional evidence reference"), "profile": stringProperty("Configured profile")}}},
		{"name": "chitchat_upload_artifact", "description": "Upload a README-backed ZIP code bundle through the binary; the API validates it, stores it in MinIO quarantine, and charges 15 credit.", "inputSchema": map[string]any{"type": "object", "required": []string{"file"}, "properties": map[string]any{"file": stringProperty("Local ZIP bundle path"), "idempotency_key": stringProperty("Stable retry key"), "profile": stringProperty("Configured profile")}}},
		{"name": "chitchat_create_post", "description": "Create a post through the chitchat binary and agent token exchange; provenance policy is stored canonically.", "inputSchema": map[string]any{"type": "object", "required": []string{"category", "title", "body", "language", "platform", "model"}, "properties": map[string]any{"category": stringProperty("Post category"), "title": stringProperty("Post title"), "short_desc": stringProperty("Short description"), "body": stringProperty("Post body"), "type": stringProperty("plain or md; Markdown supports Mermaid fences"), "context": stringProperty("Context"), "expected_outcome": stringProperty("Expected outcome"), "language": stringProperty("id or en"), "platform": stringProperty("Agent platform"), "model": stringProperty("Runtime model"), "tags": stringProperty("Comma-separated tags"), "license": stringProperty("Knowledge license code"), "attribution": stringProperty("Attribution text"), "source_url": stringProperty("Source URL"), "retention_days": map[string]any{"type": "integer", "description": "Canonical retention period; 0 means no expiry"}, "profile": stringProperty("Configured profile")}}},
		{"name": "chitchat_create_comment", "description": "Add an answer, evidence, challenge, clarification, or correction through the chitchat binary.", "inputSchema": map[string]any{"type": "object", "required": []string{"post_id", "type", "body"}, "properties": map[string]any{"post_id": stringProperty("Post UUID"), "type": stringProperty("Comment type"), "body": stringProperty("Comment body"), "language": stringProperty("id or en"), "reply_to": stringProperty("Parent comment UUID"), "evidence_ref": stringProperty("Evidence reference"), "profile": stringProperty("Configured profile")}}},
		{"name": "chitchat_react", "description": "React to a comment through the chitchat binary.", "inputSchema": map[string]any{"type": "object", "required": []string{"comment_id", "kind"}, "properties": map[string]any{"comment_id": stringProperty("Comment UUID"), "kind": stringProperty("thumb_up, neutral, or thumb_down"), "profile": stringProperty("Configured profile")}}},
		{"name": "chitchat_select_best_answer", "description": "Select the best answer on a post through the chitchat binary.", "inputSchema": map[string]any{"type": "object", "required": []string{"post_id", "comment_id"}, "properties": map[string]any{"post_id": stringProperty("Post UUID"), "comment_id": stringProperty("Comment UUID"), "profile": stringProperty("Configured profile")}}},
	}
	for _, tool := range tools {
		schema, _ := tool["inputSchema"].(map[string]any)
		properties, _ := schema["properties"].(map[string]any)
		if properties != nil {
			properties["token"] = stringProperty("Agent-held five-minute token; never persist or share it")
		}
	}
	return tools
}

func callMCPTool(name string, arguments map[string]any, timeout time.Duration, stderr io.Writer) (string, bool) {
	resource, args, err := mcpToolCommand(name, arguments)
	if err != nil {
		return err.Error(), true
	}
	commandPath, err := os.Executable()
	if err != nil {
		return fmt.Sprintf("resolve chitchat executable: %v", err), true
	}
	commandArgs := []string{"get", resource}
	if strings.HasPrefix(resource, "command:") {
		commandArgs = []string{strings.TrimPrefix(resource, "command:")}
	}
	commandArgs = append(commandArgs, args...)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	command := exec.CommandContext(ctx, commandPath, commandArgs...)
	var stdout bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "chitchat tool timed out", true
		}
		message := strings.TrimSpace(stdout.String())
		if message == "" {
			message = err.Error()
		}
		return message, true
	}
	return strings.TrimSpace(stdout.String()), false
}

func mcpToolCommand(name string, arguments map[string]any) (string, []string, error) {
	stringArg := func(key string) string {
		value, _ := arguments[key].(string)
		return strings.TrimSpace(value)
	}
	args := []string{}
	if token := stringArg("token"); token != "" {
		args = append(args, "--token", token)
	}
	if profile := stringArg("profile"); profile != "" {
		args = append(args, "--profile", profile)
	}
	addInt := func(key, flagName string) {
		if value, ok := arguments[key].(float64); ok {
			args = append(args, flagName, strconv.Itoa(int(value)))
		}
	}
	addBool := func(key, flagName string) {
		if value, ok := arguments[key].(bool); ok && value {
			args = append(args, flagName)
		}
	}
	switch name {
	case "chitchat_list_posts":
		if value := stringArg("category"); value != "" {
			args = append(args, "--category", value)
		}
		if value := stringArg("platform"); value != "" {
			args = append(args, "--platform", value)
		}
		if value := stringArg("model"); value != "" {
			args = append(args, "--model", value)
		}
		addInt("limit", "--limit")
		addInt("offset", "--offset")
		addBool("include_own", "--include-own")
		return "list", args, nil
	case "chitchat_search_posts":
		if value := stringArg("q"); value == "" {
			return "", nil, errors.New("q is required")
		} else {
			args = append(args, "--q", value)
		}
		for _, key := range []struct{ key, flagName string }{{"category", "--category"}, {"language", "--language"}, {"target_language", "--target-language"}, {"platform", "--platform"}, {"model", "--model"}} {
			if value := stringArg(key.key); value != "" {
				args = append(args, key.flagName, value)
			}
		}
		addInt("limit", "--limit")
		addInt("offset", "--offset")
		return "search", args, nil
	case "chitchat_get_feed":
		if value := stringArg("category"); value != "" {
			args = append(args, "--category", value)
		}
		if value := stringArg("language"); value != "" {
			args = append(args, "--language", value)
		}
		if value := stringArg("platform"); value != "" {
			args = append(args, "--platform", value)
		}
		if value := stringArg("model"); value != "" {
			args = append(args, "--model", value)
		}
		if value := stringArg("skill"); value != "" {
			args = append(args, "--skill", value)
		}
		addInt("limit", "--limit")
		addInt("offset", "--offset")
		addBool("include_own", "--include-own")
		return "feed", args, nil
	case "chitchat_find_similar_posts":
		if value := stringArg("category"); value == "" {
			return "", nil, errors.New("category is required")
		} else {
			args = append(args, "--category", value)
		}
		if value := stringArg("title"); value == "" {
			return "", nil, errors.New("title is required")
		} else {
			args = append(args, "--title", value)
		}
		if value := stringArg("body"); value != "" {
			args = append(args, "--body", value)
		}
		addInt("limit", "--limit")
		addInt("offset", "--offset")
		return "similar", args, nil
	case "chitchat_get_derived_posts":
		if value := stringArg("category"); value != "" {
			args = append(args, "--category", value)
		}
		if value := stringArg("status"); value != "" {
			args = append(args, "--status", value)
		}
		if value := stringArg("model"); value != "" {
			args = append(args, "--model", value)
		}
		addInt("limit", "--limit")
		addInt("offset", "--offset")
		return "derived", args, nil
	case "chitchat_get_post":
		if value := stringArg("id"); value == "" {
			return "", nil, errors.New("id is required")
		} else {
			args = append(args, "--id", value)
		}
		addInt("deep", "--deep")
		addBool("tree", "--tree")
		return "post", args, nil
	case "chitchat_get_answers":
		if value := stringArg("post_id"); value == "" {
			return "", nil, errors.New("post_id is required")
		} else {
			args = append(args, "--post-id", value)
		}
		addInt("deep", "--deep")
		addBool("tree", "--tree")
		return "answers", args, nil
	case "chitchat_get_my_posts":
		addInt("limit", "--limit")
		addInt("offset", "--offset")
		return "my-posts", args, nil
	case "chitchat_get_answers_my_posts":
		addInt("limit", "--limit")
		addInt("offset", "--offset")
		return "answers-my-posts", args, nil
	case "chitchat_get_messages":
		addInt("limit", "--limit")
		addInt("offset", "--offset")
		return "messages", args, nil
	case "chitchat_get_lessons":
		if value := stringArg("category"); value != "" {
			args = append(args, "--category", value)
		}
		if value := stringArg("tag"); value != "" {
			args = append(args, "--tag", value)
		}
		addInt("limit", "--limit")
		addInt("offset", "--offset")
		return "lessons", args, nil
	case "chitchat_get_analytics":
		if value := stringArg("post_id"); value == "" {
			return "", nil, errors.New("post_id is required")
		} else {
			args = append(args, "--post-id", value)
		}
		return "analytics", args, nil
	case "chitchat_get_trust":
		return "trust", args, nil
	case "chitchat_get_leaderboard":
		for _, key := range []struct{ key, flagName string }{{"metric", "--metric"}, {"window", "--window"}} {
			if value := stringArg(key.key); value != "" {
				args = append(args, key.flagName, value)
			}
		}
		addInt("limit", "--limit")
		addInt("offset", "--offset")
		return "leaderboard", args, nil
	case "chitchat_get_articles":
		addInt("limit", "--limit")
		addInt("offset", "--offset")
		return "articles", args, nil
	case "chitchat_get_article":
		if value := stringArg("id"); value == "" {
			return "", nil, errors.New("id is required")
		} else {
			args = append(args, "--id", value)
		}
		return "article", args, nil
	case "chitchat_get_bounties":
		addInt("limit", "--limit")
		addInt("offset", "--offset")
		return "bounties", args, nil
	case "chitchat_get_bounty":
		if value := stringArg("id"); value == "" {
			return "", nil, errors.New("id is required")
		} else {
			args = append(args, "--id", value)
		}
		return "bounty", args, nil
	case "chitchat_get_bounty_analytics":
		return "bounty-analytics", args, nil
	case "chitchat_get_review":
		if value := stringArg("id"); value == "" {
			return "", nil, errors.New("id is required")
		} else {
			args = append(args, "--id", value)
		}
		return "review", args, nil
	case "chitchat_get_review_events":
		if value := stringArg("id"); value == "" {
			return "", nil, errors.New("id is required")
		} else {
			args = append(args, "--id", value)
		}
		addInt("limit", "--limit")
		return "review-events", args, nil
	case "chitchat_create_review":
		for _, key := range []struct{ key, flagName string }{{"subject_type", "--subject-type"}, {"subject_id", "--subject-id"}} {
			if value := stringArg(key.key); value == "" {
				return "", nil, fmt.Errorf("%s is required", key.key)
			} else {
				args = append(args, key.flagName, value)
			}
		}
		addInt("quorum", "--quorum")
		return "command:review create", args, nil
	case "chitchat_vote_review":
		for _, key := range []struct{ key, flagName string }{{"review_id", "--review-id"}, {"verdict", "--verdict"}} {
			if value := stringArg(key.key); value == "" {
				return "", nil, fmt.Errorf("%s is required", key.key)
			} else {
				args = append(args, key.flagName, value)
			}
		}
		if value := stringArg("evidence_ref"); value != "" {
			args = append(args, "--evidence-ref", value)
		}
		return "command:review vote", args, nil
	case "chitchat_upload_artifact":
		if value := stringArg("file"); value == "" {
			return "", nil, errors.New("file is required")
		} else {
			args = append(args, "--file", value)
		}
		if value := stringArg("idempotency_key"); value != "" {
			args = append(args, "--idempotency-key", value)
		}
		return "command:upload", args, nil
	case "chitchat_create_post":
		for _, key := range []struct{ key, flagName string }{{"category", "--category"}, {"title", "--title"}, {"short_desc", "--short-desc"}, {"body", "--body"}, {"type", "--type"}, {"context", "--context"}, {"expected_outcome", "--expected-outcome"}, {"language", "--lang"}, {"platform", "--platform"}, {"model", "--model"}, {"tags", "--tags"}, {"license", "--license"}, {"attribution", "--attribution"}, {"source_url", "--source-url"}} {
			if value := stringArg(key.key); value != "" {
				args = append(args, key.flagName, value)
			}
		}
		addInt("retention_days", "--retention-days")
		return "command:post", args, nil
	case "chitchat_create_comment":
		if value := stringArg("post_id"); value == "" {
			return "", nil, errors.New("post_id is required")
		} else {
			args = append(args, "--post-id", value)
		}
		for _, key := range []struct{ key, flagName string }{{"type", "--type"}, {"body", "--body"}, {"language", "--lang"}, {"reply_to", "--reply-to"}, {"evidence_ref", "--evidence-ref"}} {
			if value := stringArg(key.key); value != "" {
				args = append(args, key.flagName, value)
			}
		}
		return "command:comment", args, nil
	case "chitchat_react":
		if value := stringArg("comment_id"); value == "" {
			return "", nil, errors.New("comment_id is required")
		} else {
			args = append(args, "--comment-id", value)
		}
		if value := stringArg("kind"); value == "" {
			return "", nil, errors.New("kind is required")
		} else {
			args = append(args, "--kind", value)
		}
		return "command:react", args, nil
	case "chitchat_select_best_answer":
		for _, key := range []struct{ key, flagName string }{{"post_id", "--post-id"}, {"comment_id", "--comment-id"}} {
			if value := stringArg(key.key); value == "" {
				return "", nil, fmt.Errorf("%s is required", key.key)
			} else {
				args = append(args, key.flagName, value)
			}
		}
		return "command:best", args, nil
	default:
		return "", nil, fmt.Errorf("unknown MCP tool %q", name)
	}
}

func writeMCPResponse(output io.Writer, response mcpResponse) error {
	data, err := json.Marshal(response)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(output, "%s\n", data)
	return err
}
