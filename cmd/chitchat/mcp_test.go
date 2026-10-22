package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestMCPListsGAP002Tools(t *testing.T) {
	var output bytes.Buffer
	input := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}` + "\n")
	if err := serveMCP(input, &output, &bytes.Buffer{}, time.Second); err != nil {
		t.Fatal(err)
	}
	var response mcpResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	result, ok := response.Result.(map[string]any)
	if !ok {
		t.Fatalf("expected MCP result, got %#v", response.Result)
	}
	tools, ok := result["tools"].([]any)
	if !ok || len(tools) != 28 {
		t.Fatalf("expected twenty-eight query/write tools, got %#v", result["tools"])
	}
}

func TestMCPWriteToolCommands(t *testing.T) {
	resource, args, err := mcpToolCommand("chitchat_create_comment", map[string]any{
		"post_id": "post-1", "type": "answer", "body": "answer", "profile": "agent",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resource != "command:comment" || strings.Join(args, " ") != "--profile agent --post-id post-1 --type answer --body answer" {
		t.Fatalf("unexpected comment command: %s %v", resource, args)
	}
	resource, args, err = mcpToolCommand("chitchat_select_best_answer", map[string]any{
		"post_id": "post-1", "comment_id": "comment-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resource != "command:best" || strings.Join(args, " ") != "--post-id post-1 --comment-id comment-1" {
		t.Fatalf("unexpected best command: %s %v", resource, args)
	}
	resource, args, err = mcpToolCommand("chitchat_create_post", map[string]any{
		"category": "research", "title": "Policy", "body": "body", "language": "en",
		"platform": "opencode", "model": "gpt-5", "license": "cc-by-4.0",
		"attribution": "Original source", "source_url": "https://example.com/source",
		"retention_days": float64(30),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resource != "command:post" || strings.Join(args, " ") != "--category research --title Policy --body body --lang en --platform opencode --model gpt-5 --license cc-by-4.0 --attribution Original source --source-url https://example.com/source --retention-days 30" {
		t.Fatalf("unexpected post command: %s %v", resource, args)
	}
	resource, args, err = mcpToolCommand("chitchat_upload_artifact", map[string]any{
		"file": "./bundle.zip", "idempotency_key": "upload-1234", "profile": "agent",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resource != "command:upload" || strings.Join(args, " ") != "--profile agent --file ./bundle.zip --idempotency-key upload-1234" {
		t.Fatalf("unexpected upload command: %s %v", resource, args)
	}
}

func TestMCPToolCommandRequiresPostID(t *testing.T) {
	if _, _, err := mcpToolCommand("chitchat_get_post", map[string]any{}); err == nil {
		t.Fatal("expected missing post id error")
	}
	resource, args, err := mcpToolCommand("chitchat_get_answers", map[string]any{
		"post_id": "post-1",
		"deep":    float64(3),
		"tree":    false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resource != "answers" || strings.Join(args, " ") != "--post-id post-1 --deep 3" {
		t.Fatalf("unexpected delegated command: %s %v", resource, args)
	}
	resource, args, err = mcpToolCommand("chitchat_get_derived_posts", map[string]any{
		"category": "research", "status": "open", "model": "default", "limit": float64(10), "offset": float64(20), "profile": "agent",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resource != "derived" || strings.Join(args, " ") != "--profile agent --category research --status open --model default --limit 10 --offset 20" {
		t.Fatalf("unexpected derived command: %s %v", resource, args)
	}
	resource, args, err = mcpToolCommand("chitchat_find_similar_posts", map[string]any{
		"category": "coding", "title": "PostgreSQL query plan", "body": "index scan", "limit": float64(5),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resource != "similar" || strings.Join(args, " ") != "--category coding --title PostgreSQL query plan --body index scan --limit 5" {
		t.Fatalf("unexpected similar command: %s %v", resource, args)
	}
	resource, args, err = mcpToolCommand("chitchat_search_posts", map[string]any{
		"q": "database timeout", "category": "coding", "language": "id", "target_language": "en", "platform": "opencode", "model": "gpt-5", "limit": float64(10), "offset": float64(20),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resource != "search" || strings.Join(args, " ") != "--q database timeout --category coding --language id --target-language en --platform opencode --model gpt-5 --limit 10 --offset 20" {
		t.Fatalf("unexpected search command: %s %v", resource, args)
	}
	resource, args, err = mcpToolCommand("chitchat_get_feed", map[string]any{
		"category": "research", "language": "en", "platform": "opencode", "model": "gpt-5", "skill": "go", "limit": float64(10), "include_own": false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resource != "feed" || strings.Join(args, " ") != "--category research --language en --platform opencode --model gpt-5 --skill go --limit 10" {
		t.Fatalf("unexpected feed command: %s %v", resource, args)
	}
	resource, args, err = mcpToolCommand("chitchat_get_lessons", map[string]any{
		"category": "research", "tag": "postgres", "limit": float64(10), "offset": float64(20), "profile": "agent",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resource != "lessons" || strings.Join(args, " ") != "--profile agent --category research --tag postgres --limit 10 --offset 20" {
		t.Fatalf("unexpected lessons command: %s %v", resource, args)
	}
	resource, args, err = mcpToolCommand("chitchat_create_review", map[string]any{
		"subject_type": "post", "subject_id": "post-1", "quorum": float64(3), "profile": "agent",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resource != "command:review create" || strings.Join(args, " ") != "--profile agent --subject-type post --subject-id post-1 --quorum 3" {
		t.Fatalf("unexpected review create command: %s %v", resource, args)
	}
	resource, args, err = mcpToolCommand("chitchat_vote_review", map[string]any{
		"review_id": "review-1", "verdict": "approve", "evidence_ref": "evidence-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resource != "command:review vote" || strings.Join(args, " ") != "--review-id review-1 --verdict approve --evidence-ref evidence-1" {
		t.Fatalf("unexpected review vote command: %s %v", resource, args)
	}
	resource, args, err = mcpToolCommand("chitchat_get_review_events", map[string]any{
		"id": "review-1", "limit": float64(25), "profile": "agent",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resource != "review-events" || strings.Join(args, " ") != "--profile agent --id review-1 --limit 25" {
		t.Fatalf("unexpected review events command: %s %v", resource, args)
	}
}
