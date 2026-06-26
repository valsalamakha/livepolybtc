// Package auth implements Polymarket CLOB L2 (API-key) request signing.
//
// Polymarket authenticates private REST and websocket requests with an HMAC
// signature over the concatenation of the request timestamp, HTTP method,
// request path and (optional) body. The secret is a base64url-encoded value;
// the resulting signature is base64url-encoded. The signature and the
// supporting metadata are sent as POLY_* headers.
//
// See https://docs.polymarket.us/api-reference (L2 header authentication).
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"time"
)

// Credentials are the L2 API credentials sourced from the environment.
type Credentials struct {
	APIKey     string
	Secret     string
	Passphrase string
	// Address is the maker/funder wallet address (POLY_ADDRESS). Optional for
	// signing but required by some endpoints.
	Address string
}

// Header names used by the Polymarket CLOB for L2 authentication.
const (
	HeaderAddress    = "POLY_ADDRESS"
	HeaderSignature  = "POLY_SIGNATURE"
	HeaderTimestamp  = "POLY_TIMESTAMP"
	HeaderAPIKey     = "POLY_API_KEY"
	HeaderPassphrase = "POLY_PASSPHRASE"
)

// Signer produces L2 authentication headers for CLOB requests.
type Signer struct {
	creds Credentials
	// now is injectable for deterministic tests.
	now func() time.Time
}

// NewSigner constructs a Signer from credentials.
func NewSigner(creds Credentials) *Signer {
	return &Signer{creds: creds, now: time.Now}
}

// Sign computes the HMAC signature for a request. timestamp is the unix-second
// string included in the message and returned to the caller; method is the
// upper-case HTTP verb; path is the request path (including leading slash, no
// host); body is the raw request body (empty for GET).
//
// The message signed is: timestamp + method + path + body.
func (s *Signer) Sign(timestamp, method, path, body string) (string, error) {
	secret, err := base64.URLEncoding.DecodeString(s.creds.Secret)
	if err != nil {
		// Some secrets are emitted without padding; retry with raw encoding.
		secret, err = base64.RawURLEncoding.DecodeString(s.creds.Secret)
		if err != nil {
			return "", fmt.Errorf("decode secret: %w", err)
		}
	}
	msg := timestamp + method + path + body
	mac := hmac.New(sha256.New, secret)
	if _, err := mac.Write([]byte(msg)); err != nil {
		return "", fmt.Errorf("hmac write: %w", err)
	}
	return base64.URLEncoding.EncodeToString(mac.Sum(nil)), nil
}

// Headers builds the full set of L2 headers for a request, generating a fresh
// timestamp. The returned map can be applied directly to an http.Request.
func (s *Signer) Headers(method, path, body string) (map[string]string, error) {
	ts := strconv.FormatInt(s.now().Unix(), 10)
	sig, err := s.Sign(ts, method, path, body)
	if err != nil {
		return nil, err
	}
	h := map[string]string{
		HeaderSignature:  sig,
		HeaderTimestamp:  ts,
		HeaderAPIKey:     s.creds.APIKey,
		HeaderPassphrase: s.creds.Passphrase,
	}
	if s.creds.Address != "" {
		h[HeaderAddress] = s.creds.Address
	}
	return h, nil
}

// HasCredentials reports whether the minimum L2 credentials are present.
func (s *Signer) HasCredentials() bool {
	return s.creds.APIKey != "" && s.creds.Secret != "" && s.creds.Passphrase != ""
}
