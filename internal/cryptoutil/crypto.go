package cryptoutil

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// LoadOrCreateKey derives a 32-byte key from envKey or persists a local master key in dataDir.
func LoadOrCreateKey(dataDir, envKey string) ([32]byte, error) {
	if trimmed := strings.TrimSpace(envKey); trimmed != "" {
		return sha256.Sum256([]byte(trimmed)), nil
	}

	var out [32]byte
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return out, fmt.Errorf("create data directory: %w", err)
	}
	keyPath := filepath.Join(dataDir, ".master.key")
	if raw, err := os.ReadFile(keyPath); err == nil {
		decoded, decErr := hex.DecodeString(strings.TrimSpace(string(raw)))
		if decErr == nil && len(decoded) == 32 {
			copy(out[:], decoded)
			return out, nil
		}
	}

	if _, err := rand.Read(out[:]); err != nil {
		return out, fmt.Errorf("generate master key: %w", err)
	}
	encoded := hex.EncodeToString(out[:]) + "\n"
	tmp := keyPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(encoded), 0o600); err != nil {
		return out, fmt.Errorf("write master key: %w", err)
	}
	if err := os.Rename(tmp, keyPath); err != nil {
		_ = os.Remove(tmp)
		return out, fmt.Errorf("commit master key: %w", err)
	}
	return out, nil
}

// Encrypt encrypts plaintext using AES-256-GCM and returns a base64 token.
func Encrypt(key [32]byte, plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return "v1:" + base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt reverses Encrypt for a base64 AES-256-GCM token.
func Decrypt(key [32]byte, token string) (string, error) {
	if token == "" {
		return "", nil
	}
	rawStr := strings.TrimPrefix(token, "v1:")
	raw, err := base64.StdEncoding.DecodeString(rawStr)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, ciphertext := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
