package devicekey

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGenerateWritesProtectedKeyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "device-key.json")
	summary, err := Generate(path, time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if summary.PublicKey == "" || summary.Fingerprint == "" {
		t.Fatal("expected public key and fingerprint")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("expected mode 0600, got %o", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var key File
	if err := json.Unmarshal(data, &key); err != nil {
		t.Fatal(err)
	}
	if key.Version != fileVersion || key.Algorithm != "Ed25519" {
		t.Fatalf("unexpected key metadata: %+v", key)
	}
	if key.PublicKey != summary.PublicKey || key.Fingerprint != summary.Fingerprint {
		t.Fatal("summary does not match key file")
	}
}
