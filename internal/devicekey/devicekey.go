package devicekey

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const fileVersion = 1

type File struct {
	Version     int    `json:"version"`
	Algorithm   string `json:"algorithm"`
	PrivateKey  string `json:"private_key"`
	PublicKey   string `json:"public_key"`
	Fingerprint string `json:"fingerprint"`
	CreatedAt   string `json:"created_at"`
}

type Summary struct {
	Path        string `json:"path"`
	PublicKey   string `json:"public_key"`
	Fingerprint string `json:"fingerprint"`
}

func Load(path string) (File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return File{}, fmt.Errorf("read device key: %w", err)
	}
	var file File
	if err := json.Unmarshal(data, &file); err != nil {
		return File{}, fmt.Errorf("decode device key: %w", err)
	}
	if file.Algorithm != "Ed25519" || file.PrivateKey == "" || file.PublicKey == "" || file.Fingerprint == "" {
		return File{}, errors.New("device key file is incomplete")
	}
	return file, nil
}

func Generate(path string, now time.Time) (Summary, error) {
	if path == "" {
		return Summary{}, errors.New("device key output path is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return Summary{}, fmt.Errorf("generate Ed25519 key: %w", err)
	}
	file := File{
		Version:     fileVersion,
		Algorithm:   "Ed25519",
		PrivateKey:  base64.RawStdEncoding.EncodeToString(privateKey),
		PublicKey:   base64.RawStdEncoding.EncodeToString(publicKey),
		Fingerprint: Fingerprint(publicKey),
		CreatedAt:   now.UTC().Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return Summary{}, fmt.Errorf("encode device key: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return Summary{}, fmt.Errorf("create device key directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".device-key-*")
	if err != nil {
		return Summary{}, fmt.Errorf("create device key file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return Summary{}, fmt.Errorf("protect device key file: %w", err)
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return Summary{}, fmt.Errorf("write device key file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return Summary{}, fmt.Errorf("close device key file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return Summary{}, fmt.Errorf("save device key file: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return Summary{}, fmt.Errorf("protect saved device key file: %w", err)
	}
	return Summary{Path: path, PublicKey: file.PublicKey, Fingerprint: file.Fingerprint}, nil
}

func Fingerprint(publicKey ed25519.PublicKey) string {
	digest := sha256.Sum256(publicKey)
	return "sha256:" + hex.EncodeToString(digest[:])
}
