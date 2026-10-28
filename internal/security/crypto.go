package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
)

func DeriveKey(master []byte, label string) []byte {
	h := hmac.New(sha256.New, master)
	h.Write([]byte(label))
	return h.Sum(nil)
}

func LoadOrCreateSecret(dataDir, fromEnv string) ([]byte, error) {
	if v := strings.TrimSpace(fromEnv); v != "" {
		return []byte(v), nil
	}
	path := filepath.Join(dataDir, ".secret")
	if b, err := os.ReadFile(path); err == nil {
		if s := strings.TrimSpace(string(b)); s != "" {
			return []byte(s), nil
		}
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	secret, err := RandomString(48)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(secret), 0o600); err != nil {
		return nil, fmt.Errorf("write secret: %w", err)
	}
	return []byte(secret), nil
}

type Cipher struct {
	aead cipher.AEAD
}

func NewCipher(master []byte) (*Cipher, error) {
	key := DeriveKey(master, "aes")
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

func (c *Cipher) Encrypt(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	out := c.aead.Seal(nonce, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(out), nil
}

func (c *Cipher) Decrypt(enc string) (string, error) {
	if enc == "" {
		return "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", fmt.Errorf("decode secret: %w", err)
	}
	ns := c.aead.NonceSize()
	if len(raw) < ns {
		return "", fmt.Errorf("invalid cipher text")
	}
	plain, err := c.aead.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return "", fmt.Errorf("decrypt secret: %w", err)
	}
	return string(plain), nil
}

const passwordAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789@#$%"

func RandomString(n int) (string, error) {
	var sb strings.Builder
	for i := 0; i < n; i++ {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(passwordAlphabet))))
		if err != nil {
			return "", err
		}
		sb.WriteByte(passwordAlphabet[idx.Int64()])
	}
	return sb.String(), nil
}

func MaskKey(enc string) string {
	if enc == "" {
		return ""
	}
	if len(enc) <= 4 {
		return "****"
	}
	return "****" + enc[len(enc)-4:]
}
