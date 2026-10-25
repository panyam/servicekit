package webhooks

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"
)

// The example from the Standard Webhooks specification and the test suites of
// its reference libraries. A signer that only round-trips with its own
// verifier can't catch a wrong key derivation; this vector can (mcpkit shipped
// one for four months, mcpkit issue 1574).
const (
	specSecret    = "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"
	specMsgID     = "msg_p5jXN8AQM9LWM0D4loKWxJek"
	specTimestamp = "1614265330"
	specBody      = `{"test": 2432232314}`
	specSignature = "v1,g0hM9SsE+OTPJTGt/tmIKtSyZlE3uFJELVlNIOLJ1OE="
)

func TestSign_MatchesSpecVector(t *testing.T) {
	if got := Sign(specSecret, specMsgID, specTimestamp, []byte(specBody)); got != specSignature {
		t.Fatalf("Sign = %q, want %q", got, specSignature)
	}
	if !Verify(specSecret, specMsgID, specTimestamp, []byte(specBody), specSignature) {
		t.Fatal("Verify rejected the spec vector")
	}
}

func TestSign_MatchesFormula(t *testing.T) {
	raw := []byte("0123456789abcdef0123456789abcdef")
	body := []byte(`{"eventId":"evt_1"}`)
	mac := hmac.New(sha256.New, raw)
	mac.Write([]byte("evt_1.1700000000."))
	mac.Write(body)
	want := "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))

	for _, secret := range []string{
		SecretPrefix + base64.StdEncoding.EncodeToString(raw),
		SecretPrefix + base64.RawURLEncoding.EncodeToString(raw),
	} {
		if got := Sign(secret, "evt_1", "1700000000", body); got != want {
			t.Errorf("Sign(%q) = %q, want %q (key must be the decoded bytes)", secret, got, want)
		}
	}
}

func TestSigningKey_DecodesOnlyPrefixedSecrets(t *testing.T) {
	raw := []byte("0123456789abcdef0123456789abcdef")
	for _, enc := range []string{
		base64.StdEncoding.EncodeToString(raw),
		base64.RawURLEncoding.EncodeToString(raw),
	} {
		if got := signingKey(SecretPrefix + enc); !bytes.Equal(got, raw) {
			t.Errorf("signingKey(whsec_%s) = %q, want the decoded bytes", enc, got)
		}
	}
	if got := signingKey("cm90YXRlZA=="); string(got) != "cm90YXRlZA==" {
		t.Errorf("an unprefixed secret must be used verbatim even when it is valid base64, got %q", got)
	}
	bad := SecretPrefix + "!!not base64!!"
	if got := signingKey(bad); string(got) != bad {
		t.Errorf("a prefixed value that can't decode must fall back to its raw bytes, got %q", got)
	}
}

func TestVerify(t *testing.T) {
	secret := GenerateSecret()
	body := []byte(`{"n":1}`)
	ts := Timestamp(time.Unix(1700000000, 0))
	sig := Sign(secret, "msg_1", ts, body)

	cases := []struct {
		name      string
		secret    string
		msgID, ts string
		body      []byte
		signature string
		want      bool
	}{
		{"exact", secret, "msg_1", ts, body, sig, true},
		{"one of several", secret, "msg_1", ts, body, "v1,AAAA " + sig + " v2,BBBB", true},
		{"other secret", GenerateSecret(), "msg_1", ts, body, sig, false},
		{"other id", secret, "msg_2", ts, body, sig, false},
		{"other timestamp", secret, "msg_1", "1700000001", body, sig, false},
		{"other body", secret, "msg_1", ts, []byte(`{"n":2}`), sig, false},
		{"no version prefix", secret, "msg_1", ts, body, strings.TrimPrefix(sig, "v1,"), false},
		{"empty", secret, "msg_1", ts, body, "", false},
	}
	for _, c := range cases {
		if got := Verify(c.secret, c.msgID, c.ts, c.body, c.signature); got != c.want {
			t.Errorf("%s: Verify = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestGenerateSecret(t *testing.T) {
	a, b := GenerateSecret(), GenerateSecret()
	if a == b {
		t.Fatal("two generated secrets are equal")
	}
	if err := ValidateSecret(a); err != nil {
		t.Fatalf("generated secret fails validation: %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(a, SecretPrefix))
	if err != nil {
		t.Fatalf("generated secret must be standard padded base64 for the reference verifiers: %v", err)
	}
	if len(raw) != 32 {
		t.Errorf("generated secret holds %d bytes, want 32", len(raw))
	}
}

func TestValidateSecret(t *testing.T) {
	enc := func(n int) string { return base64.StdEncoding.EncodeToString(make([]byte, n)) }
	cases := []struct {
		name   string
		secret string
		want   error
	}{
		{"24 bytes", SecretPrefix + enc(24), nil},
		{"64 bytes", SecretPrefix + enc(64), nil},
		{"url-safe", SecretPrefix + base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0xfb}, 32)), nil},
		{"spec vector", specSecret, nil},
		{"no prefix", enc(32), ErrSecretPrefix},
		{"prefix only", SecretPrefix, ErrSecretEmpty},
		{"not base64", SecretPrefix + "!!!!", ErrSecretEncoding},
		{"23 bytes", SecretPrefix + enc(23), ErrSecretTooShort},
		{"65 bytes", SecretPrefix + enc(65), ErrSecretTooLong},
	}
	for _, c := range cases {
		if err := ValidateSecret(c.secret); !errors.Is(err, c.want) {
			t.Errorf("%s: ValidateSecret = %v, want %v", c.name, err, c.want)
		}
	}
}

func TestTimestampWithinTolerance(t *testing.T) {
	now := time.Unix(1700000000, 0)
	cases := []struct {
		ts   string
		want bool
	}{
		{"1700000000", true},
		{"1699999700", true},  // 5m old
		{"1700000300", true},  // 5m ahead
		{"1699999699", false}, // just over
		{"1700000301", false},
		{" 1700000000 ", true},
		{"", false},
		{"soon", false},
	}
	for _, c := range cases {
		if got := TimestampWithinTolerance(c.ts, now, DefaultTolerance); got != c.want {
			t.Errorf("TimestampWithinTolerance(%q) = %v, want %v", c.ts, got, c.want)
		}
	}
}
