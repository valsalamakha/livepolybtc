package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"testing"
	"time"
)

func TestSignMatchesReferenceHMAC(t *testing.T) {
	rawSecret := []byte("super-secret-key")
	encSecret := base64.URLEncoding.EncodeToString(rawSecret)

	s := NewSigner(Credentials{APIKey: "k", Secret: encSecret, Passphrase: "p"})

	ts, method, path, body := "1700000000", "POST", "/order", `{"a":1}`
	got, err := s.Sign(ts, method, path, body)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	mac := hmac.New(sha256.New, rawSecret)
	mac.Write([]byte(ts + method + path + body))
	want := base64.URLEncoding.EncodeToString(mac.Sum(nil))

	if got != want {
		t.Errorf("signature mismatch:\n got=%s\nwant=%s", got, want)
	}
}

func TestHeadersIncludeAllFields(t *testing.T) {
	encSecret := base64.URLEncoding.EncodeToString([]byte("s"))
	s := NewSigner(Credentials{APIKey: "key", Secret: encSecret, Passphrase: "pass", Address: "0xabc"})
	s.now = func() time.Time { return time.Unix(1700000000, 0) }

	h, err := s.Headers("GET", "/balance", "")
	if err != nil {
		t.Fatalf("headers: %v", err)
	}
	for _, k := range []string{HeaderSignature, HeaderTimestamp, HeaderAPIKey, HeaderPassphrase, HeaderAddress} {
		if h[k] == "" {
			t.Errorf("missing header %s", k)
		}
	}
	if h[HeaderTimestamp] != "1700000000" {
		t.Errorf("timestamp = %s, want 1700000000", h[HeaderTimestamp])
	}
	if h[HeaderAPIKey] != "key" {
		t.Errorf("api key header wrong: %s", h[HeaderAPIKey])
	}
}

func TestSignRejectsBadSecret(t *testing.T) {
	s := NewSigner(Credentials{Secret: "!!!not base64!!!"})
	if _, err := s.Sign("1", "GET", "/", ""); err == nil {
		t.Error("expected error for invalid base64 secret")
	}
}

func TestHasCredentials(t *testing.T) {
	full := NewSigner(Credentials{APIKey: "a", Secret: "b", Passphrase: "c"})
	if !full.HasCredentials() {
		t.Error("expected HasCredentials true")
	}
	partial := NewSigner(Credentials{APIKey: "a"})
	if partial.HasCredentials() {
		t.Error("expected HasCredentials false")
	}
}
