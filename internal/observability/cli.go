package observability

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"time"
)

type commandEvent struct {
	Event      string  `json:"event"`
	Command    string  `json:"command"`
	DurationMS float64 `json:"duration_ms"`
	Success    bool    `json:"success"`
	ErrorClass string  `json:"error_class,omitempty"`
}

// Record writes one bounded, privacy-safe command event to stderr when CLI
// observability is enabled. Stdout remains reserved for machine-readable
// command output.
func Record(args []string, started time.Time, commandErr error, output io.Writer) {
	if !enabled() || output == nil {
		return
	}
	event := commandEvent{
		Event:      "chitchat_cli_command",
		Command:    commandName(args),
		DurationMS: float64(time.Since(started)) / float64(time.Millisecond),
		Success:    commandErr == nil,
	}
	if commandErr != nil {
		event.ErrorClass = "command_error"
	}
	_ = json.NewEncoder(output).Encode(event)
}

func enabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("CHITCHAT_OBSERVABILITY")))
	return value == "1" || value == "true" || value == "yes"
}

func commandName(args []string) string {
	if len(args) == 0 {
		return "help"
	}
	if args[0] == "--help" || args[0] == "-h" {
		return "help"
	}
	switch args[0] {
	case "device-key", "pair", "token", "post", "comment", "react", "best", "message",
		"convert-score", "watch", "preferences", "profile", "mcp", "get", "observability":
		if args[0] == "observability" && len(args) > 1 && strings.TrimSpace(args[1]) != "" {
			return "observability." + args[1]
		}
		if args[0] == "get" && len(args) > 1 && strings.TrimSpace(args[1]) != "" {
			for _, resource := range []string{"me", "score", "credit", "messages", "lessons", "post", "comments", "list", "search", "feed", "derived", "my-posts", "answers-my-posts", "answers", "analytics", "category", "categories", "tags", "similar", "find-task", "find-nearest", "preferences"} {
				if args[1] == resource {
					return "get." + resource
				}
			}
			return "get.unknown"
		}
		return args[0]
	default:
		return "unknown"
	}
}
