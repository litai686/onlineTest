package security

import (
	"testing"
)

func TestJWTIssueParse(t *testing.T) {
	m := NewJWTManager([]byte("test-secret"), 30)
	token, exp, err := m.Issue(42, "teacher", "王老师", 3)
	if err != nil {
		t.Fatal(err)
	}
	if token == "" || exp.IsZero() {
		t.Fatal("empty token/expiry")
	}
	claims, err := m.Parse(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != "42" || claims.Role != "teacher" || claims.Ver != 3 || claims.Name != "王老师" {
		t.Fatalf("unexpected claims %+v", claims)
	}

	other := NewJWTManager([]byte("other-secret"), 30)
	if _, err := other.Parse(token); err == nil {
		t.Fatal("token signed with another key should fail")
	}
	if _, err := m.Parse(token + "x"); err == nil {
		t.Fatal("tampered token should fail")
	}
}

func TestCipherRoundTrip(t *testing.T) {
	c, err := NewCipher([]byte("master-secret"))
	if err != nil {
		t.Fatal(err)
	}
	enc, err := c.Encrypt("sk-1234567890")
	if err != nil {
		t.Fatal(err)
	}
	if enc == "" || enc == "sk-1234567890" {
		t.Fatal("cipher text not encrypted")
	}
	plain, err := c.Decrypt(enc)
	if err != nil {
		t.Fatal(err)
	}
	if plain != "sk-1234567890" {
		t.Fatalf("round trip mismatch: %q", plain)
	}
	if _, err := c.Decrypt(enc + "AAAA"); err == nil {
		t.Fatal("tampered cipher text should fail")
	}
	if enc, _ := c.Encrypt(""); enc != "" {
		t.Fatal("empty plaintext should produce empty cipher")
	}
}

func TestPassword(t *testing.T) {
	hash, err := HashPassword("Student123")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "Student123" {
		t.Fatal("password not hashed")
	}
	if !CheckPassword(hash, "Student123") {
		t.Fatal("valid password rejected")
	}
	if CheckPassword(hash, "wrong") {
		t.Fatal("invalid password accepted")
	}
}

func TestMaskKey(t *testing.T) {
	if got := MaskKey("sk-abcdefgh"); got != "****efgh" {
		t.Fatalf("MaskKey = %q", got)
	}
	if MaskKey("") != "" {
		t.Fatal("empty key should be empty")
	}
}
