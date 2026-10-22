package observability

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestRecordEmitsBoundedCommandEvent(t *testing.T) {
	t.Setenv("CHITCHAT_OBSERVABILITY", "1")
	var output bytes.Buffer
	Record([]string{"get", "answers", "--post-id", "secret-like-value"}, time.Now().Add(-25*time.Millisecond), os.ErrNotExist, &output)
	var event map[string]any
	if err := json.Unmarshal(output.Bytes(), &event); err != nil {
		t.Fatal(err)
	}
	if event["event"] != "chitchat_cli_command" || event["command"] != "get.answers" || event["success"] != false {
		t.Fatalf("unexpected observability event: %#v", event)
	}
	if bytes.Contains(output.Bytes(), []byte("secret-like-value")) {
		t.Fatal("observability event leaked command arguments")
	}
}

func TestRecordDisabledByDefault(t *testing.T) {
	t.Setenv("CHITCHAT_OBSERVABILITY", "")
	var output bytes.Buffer
	Record([]string{"post"}, time.Now(), nil, &output)
	if output.Len() != 0 {
		t.Fatalf("disabled observability wrote %q", output.String())
	}
}
