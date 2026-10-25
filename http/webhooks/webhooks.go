// Package webhooks signs and verifies HTTP webhook deliveries in the Standard
// Webhooks v1 format (https://standardwebhooks.com/): the webhook-id,
// webhook-timestamp and webhook-signature headers, an HMAC-SHA256 over
// id + "." + timestamp + "." + body, and whsec_ secrets.
//
// It knows nothing about what is being delivered. Callers build the body, pick
// the message id (the receiver dedups on it, so keep it stable across retries
// of the same delivery) and set the headers. It moved here from mcpkit's
// experimental/ext/events (mcpkit issue 1578), which keeps its own names as
// wrappers.
package webhooks

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"
)

// Header names for a Standard Webhooks delivery. HTTP header names are
// case-insensitive; these are the lower-case forms the specification uses.
const (
	HeaderID        = "webhook-id"
	HeaderTimestamp = "webhook-timestamp"
	HeaderSignature = "webhook-signature"
)

// SecretPrefix starts every spec-format signing secret. The rest of the secret
// is base64 of the raw key bytes.
const SecretPrefix = "whsec_"

// Secret length bounds, in raw bytes after base64 decoding. 24 bytes is 192
// bits.
const (
	MinSecretBytes = 24
	MaxSecretBytes = 64
)

// DefaultTolerance is how far a webhook timestamp may sit from the receiver's
// clock, either way, and still be accepted. Five minutes matches the Standard
// Webhooks reference libraries.
const DefaultTolerance = 5 * time.Minute

// Errors returned by ValidateSecret. Their messages are written to be shown
// to whoever supplied the secret, e.g. as an invalid-params message.
var (
	ErrSecretPrefix   = errors.New("must start with the whsec_ prefix")
	ErrSecretEmpty    = errors.New("missing the random portion after whsec_")
	ErrSecretEncoding = errors.New("portion after whsec_ must be base64-encoded")
	ErrSecretTooShort = errors.New("must decode to at least 24 random bytes (192 bits)")
	ErrSecretTooLong  = errors.New("must decode to at most 64 random bytes (512 bits)")
)

// GenerateSecret returns a new whsec_ secret holding 32 random bytes, encoded
// as standard padded base64 (RFC 4648 §4). That is the alphabet the reference
// verifier libraries decode; they can't read URL-safe secrets.
func GenerateSecret() string {
	var buf [32]byte
	rand.Read(buf[:]) // never fails since Go 1.24
	return SecretPrefix + base64.StdEncoding.EncodeToString(buf[:])
}

// ValidateSecret checks that s is a spec-format secret: whsec_ followed by
// base64 that decodes to MinSecretBytes to MaxSecretBytes bytes. Both standard
// and raw URL-safe base64 are accepted, since some clients emit the latter.
// It returns nil or one of the ErrSecret* errors.
//
// A server should run this when it accepts a secret, rather than store one
// that can never produce a delivery the receiver can verify.
func ValidateSecret(s string) error {
	if !strings.HasPrefix(s, SecretPrefix) {
		return ErrSecretPrefix
	}
	body := s[len(SecretPrefix):]
	if body == "" {
		return ErrSecretEmpty
	}
	decoded, err := base64.StdEncoding.DecodeString(body)
	if err != nil {
		decoded, err = base64.RawURLEncoding.DecodeString(body)
		if err != nil {
			return ErrSecretEncoding
		}
	}
	if len(decoded) < MinSecretBytes {
		return ErrSecretTooShort
	}
	if len(decoded) > MaxSecretBytes {
		return ErrSecretTooLong
	}
	return nil
}

// Timestamp formats t as a webhook-timestamp value (unix seconds).
func Timestamp(t time.Time) string {
	return strconv.FormatInt(t.Unix(), 10)
}

// Sign returns the webhook-signature value ("v1,<base64>") for one delivery.
// timestamp is the exact webhook-timestamp header value, usually
// Timestamp(time.Now()).
//
// The HMAC key is the base64-decoded bytes after the whsec_ prefix, as the
// specification says, never the literal string. A secret without the prefix
// is used verbatim, and a prefixed one that won't decode falls back to its raw
// bytes, so a stored legacy secret keeps signing instead of silently stopping
// deliveries. ValidateSecret is where malformed secrets get refused.
func Sign(secret, msgID, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, signingKey(secret))
	mac.Write([]byte(msgID))
	mac.Write([]byte("."))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	return "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// Verify reports whether signature, a webhook-signature header value, holds a
// v1 signature of this delivery under secret. The header may carry several
// space-separated signatures (a sender mid-rotation signs with both secrets);
// any match is accepted. The comparison is constant-time.
//
// Verify checks only the HMAC, so a captured delivery verifies forever.
// Receivers should also check TimestampWithinTolerance and dedup on msgID.
func Verify(secret, msgID, timestamp string, body []byte, signature string) bool {
	expected := []byte(Sign(secret, msgID, timestamp, body))
	for _, candidate := range strings.Fields(signature) {
		if hmac.Equal([]byte(candidate), expected) {
			return true
		}
	}
	return false
}

// TimestampWithinTolerance reports whether timestamp (unix seconds) is within
// tolerance of now, in either direction. An empty or unparseable timestamp is
// outside every tolerance.
func TimestampWithinTolerance(timestamp string, now time.Time, tolerance time.Duration) bool {
	n, err := strconv.ParseInt(strings.TrimSpace(timestamp), 10, 64)
	if err != nil {
		return false
	}
	d := now.Sub(time.Unix(n, 0))
	return d <= tolerance && d >= -tolerance
}

// signingKey decodes only whsec_-prefixed values. Decoding anything else would
// reinterpret any short string that happens to be valid base64, which is most
// of them.
func signingKey(secret string) []byte {
	if !strings.HasPrefix(secret, SecretPrefix) {
		return []byte(secret)
	}
	body := secret[len(SecretPrefix):]
	if decoded, err := base64.StdEncoding.DecodeString(body); err == nil {
		return decoded
	}
	if decoded, err := base64.RawURLEncoding.DecodeString(body); err == nil {
		return decoded
	}
	return []byte(secret)
}
