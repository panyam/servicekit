package webhooks

import (
	"crypto"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"strings"
	"testing"
)

// The v2a example from the Standard Webhooks specification at commit 3d65b182
// (standard-webhooks PR 372).
const (
	specV2aPrivate  = "whsk_4uKPjji/dhB8w6GW3YYDnIMYvXcdHrvH6lidTr4Z+EY="
	specV2aPublic   = "whpk_uWKK13PfwqyOgrqT0OrWKcS2uVD7e89+oUKnkoPjx7w="
	specV2aEndpoint = "ep_2fVZdC8Kx9TR4ZoCqHWt3NnLgRb"
	specV2aMsgID    = "msg_2KWPBgLlAfxdpx2AI54pPJ85f4W"
	specV2aTS       = "1674087231"
	specV2aBody     = `{"type":"contact.created","timestamp":"2022-11-03T20:26:10.344522Z","data":{"id":"1f81eb52-5198-4599-803e-771906343485"}}`
	specV2aDigest   = "YXCi5iN+rjB+iEb6bvg1XbWbVBL+r2S+as3k3n1Ih/A="
	specV2aSig      = "v2a,kG9XaxZjAEpKB8KeQQb5G1Xp73YH5Rf5gxoWe7cg1kYMiyspdtZXe9w+dAwKeOurAwxfqmlOG/QR4m9s37DzAg=="
)

func mustPriv(t *testing.T, s string) ed25519.PrivateKey {
	t.Helper()
	k, err := ParsePrivateKey(s)
	if err != nil {
		t.Fatalf("ParsePrivateKey(%q): %v", s, err)
	}
	return k
}

func mustPub(t *testing.T, s string) ed25519.PublicKey {
	t.Helper()
	k, err := ParsePublicKey(s)
	if err != nil {
		t.Fatalf("ParsePublicKey(%q): %v", s, err)
	}
	return k
}

func TestV2a_MatchesSpecVector(t *testing.T) {
	priv, pub := mustPriv(t, specV2aPrivate), mustPub(t, specV2aPublic)
	if got := EncodePublicKey(priv.Public().(ed25519.PublicKey)); got != specV2aPublic {
		t.Fatalf("public key from the spec's private key = %q, want %q", got, specV2aPublic)
	}
	if got := EncodePrivateKey(priv); got != specV2aPrivate {
		t.Fatalf("EncodePrivateKey round trip = %q, want %q", got, specV2aPrivate)
	}
	digest := v2aDigest(specV2aEndpoint, specV2aMsgID, specV2aTS, []byte(specV2aBody))
	if got := base64.StdEncoding.EncodeToString(digest[:]); got != specV2aDigest {
		t.Fatalf("digest = %q, want %q", got, specV2aDigest)
	}
	got, err := SignV2a(priv, specV2aEndpoint, specV2aMsgID, specV2aTS, []byte(specV2aBody))
	if err != nil {
		t.Fatal(err)
	}
	if got != specV2aSig {
		t.Fatalf("SignV2a = %q, want %q", got, specV2aSig)
	}
	header := "v1,K5oZfzN95Z9UVu1EsfQmfVNQhnkZ2pj9o9NDN/H/pI4= " + specV2aSig
	if !VerifyV2a([]ed25519.PublicKey{pub}, specV2aEndpoint, specV2aMsgID, specV2aTS, []byte(specV2aBody), header) {
		t.Fatal("VerifyV2a rejected the spec vector")
	}
}

func TestVerifyV2a(t *testing.T) {
	priv, pub := mustPriv(t, specV2aPrivate), mustPub(t, specV2aPublic)
	_, otherPub := GenerateKeyPair()
	other := mustPub(t, otherPub)
	body := []byte(specV2aBody)
	sig, err := SignV2a(priv, specV2aEndpoint, specV2aMsgID, specV2aTS, body)
	if err != nil {
		t.Fatal(err)
	}
	v1a, err := SignV1a(priv, specV2aMsgID, specV2aTS, body)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name       string
		keys       []ed25519.PublicKey
		endpointID string
		msgID, ts  string
		body       []byte
		signature  string
		want       bool
	}{
		{"exact", []ed25519.PublicKey{pub}, specV2aEndpoint, specV2aMsgID, specV2aTS, body, sig, true},
		{"rotation: second key matches", []ed25519.PublicKey{other, pub}, specV2aEndpoint, specV2aMsgID, specV2aTS, body, sig, true},
		{"untrusted key", []ed25519.PublicKey{other}, specV2aEndpoint, specV2aMsgID, specV2aTS, body, sig, false},
		{"no keys", nil, specV2aEndpoint, specV2aMsgID, specV2aTS, body, sig, false},
		{"replayed to another endpoint", []ed25519.PublicKey{pub}, "ep_other", specV2aMsgID, specV2aTS, body, sig, false},
		{"endpoint ID with a dot", []ed25519.PublicKey{pub}, "ep.x", specV2aMsgID, specV2aTS, body, sig, false},
		{"other id", []ed25519.PublicKey{pub}, specV2aEndpoint, "msg_2", specV2aTS, body, sig, false},
		{"other timestamp", []ed25519.PublicKey{pub}, specV2aEndpoint, specV2aMsgID, "1674087232", body, sig, false},
		{"other body", []ed25519.PublicKey{pub}, specV2aEndpoint, specV2aMsgID, specV2aTS, []byte(`{}`), sig, false},
		{"v2a body under a v1a label", []ed25519.PublicKey{pub}, specV2aEndpoint, specV2aMsgID, specV2aTS, body, "v1a," + strings.TrimPrefix(sig, "v2a,"), false},
		{"only a v1a entry", []ed25519.PublicKey{pub}, specV2aEndpoint, specV2aMsgID, specV2aTS, body, v1a, false},
		{"bad base64 then good", []ed25519.PublicKey{pub}, specV2aEndpoint, specV2aMsgID, specV2aTS, body, "v2a,!!! " + sig, true},
		{"empty", []ed25519.PublicKey{pub}, specV2aEndpoint, specV2aMsgID, specV2aTS, body, "", false},
	}
	for _, c := range cases {
		if got := VerifyV2a(c.keys, c.endpointID, c.msgID, c.ts, c.body, c.signature); got != c.want {
			t.Errorf("%s: VerifyV2a = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestSignV2a_RejectsBadEndpointID(t *testing.T) {
	priv := mustPriv(t, specV2aPrivate)
	for _, id := range []string{"", "ep.1"} {
		if _, err := SignV2a(priv, id, "msg_1", "1", nil); !errors.Is(err, ErrEndpointID) {
			t.Errorf("SignV2a(endpoint %q) error = %v, want ErrEndpointID", id, err)
		}
	}
}

// digestOnlySigner stands in for a KMS signer: it records what it was handed
// and refuses anything longer than a digest.
type digestOnlySigner struct {
	key  ed25519.PrivateKey
	seen []byte
}

func (s *digestOnlySigner) Public() crypto.PublicKey { return s.key.Public() }

func (s *digestOnlySigner) Sign(r io.Reader, msg []byte, opts crypto.SignerOpts) ([]byte, error) {
	s.seen = msg
	if len(msg) > sha256.Size {
		return nil, errors.New("message too large for the KMS")
	}
	return s.key.Sign(r, msg, opts)
}

func TestSignV2a_HandsSignerTheDigest(t *testing.T) {
	signer := &digestOnlySigner{key: mustPriv(t, specV2aPrivate)}
	big := []byte(strings.Repeat("x", 8192))
	got, err := SignV2a(signer, specV2aEndpoint, "msg_1", "1", big)
	if err != nil {
		t.Fatalf("SignV2a through a digest-only signer: %v", err)
	}
	if len(signer.seen) != sha256.Size {
		t.Fatalf("signer got %d bytes, want the %d-byte digest", len(signer.seen), sha256.Size)
	}
	if !VerifyV2a([]ed25519.PublicKey{mustPub(t, specV2aPublic)}, specV2aEndpoint, "msg_1", "1", big, got) {
		t.Fatal("signature from the digest-only signer does not verify")
	}
	if _, err := SignV1a(signer, "msg_1", "1", big); err == nil {
		t.Fatal("v1a hands the signer the full content, so a capped signer must fail")
	}
}

func TestV1a_MatchesFormula(t *testing.T) {
	priv := mustPriv(t, specV2aPrivate)
	body := []byte(`{"n":1}`)
	want := "v1a," + base64.StdEncoding.EncodeToString(ed25519.Sign(priv, []byte(`msg_1.1700000000.{"n":1}`)))
	got, err := SignV1a(priv, "msg_1", "1700000000", body)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("SignV1a = %q, want %q", got, want)
	}
}

func TestVerifyV1a(t *testing.T) {
	priv, pub := mustPriv(t, specV2aPrivate), mustPub(t, specV2aPublic)
	_, otherPub := GenerateKeyPair()
	other := mustPub(t, otherPub)
	body := []byte(`{"n":1}`)
	sig, err := SignV1a(priv, "msg_1", "1700000000", body)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name      string
		keys      []ed25519.PublicKey
		msgID, ts string
		body      []byte
		signature string
		want      bool
	}{
		{"exact", []ed25519.PublicKey{pub}, "msg_1", "1700000000", body, sig, true},
		{"beside v1", []ed25519.PublicKey{pub}, "msg_1", "1700000000", body, "v1,AAAA " + sig, true},
		{"rotation: second key matches", []ed25519.PublicKey{other, pub}, "msg_1", "1700000000", body, sig, true},
		{"untrusted key", []ed25519.PublicKey{other}, "msg_1", "1700000000", body, sig, false},
		{"other id", []ed25519.PublicKey{pub}, "msg_2", "1700000000", body, sig, false},
		{"other timestamp", []ed25519.PublicKey{pub}, "msg_1", "1700000001", body, sig, false},
		{"other body", []ed25519.PublicKey{pub}, "msg_1", "1700000000", []byte(`{"n":2}`), sig, false},
		{"labelled v2a", []ed25519.PublicKey{pub}, "msg_1", "1700000000", body, "v2a," + strings.TrimPrefix(sig, "v1a,"), false},
		{"empty", []ed25519.PublicKey{pub}, "msg_1", "1700000000", body, "", false},
	}
	for _, c := range cases {
		if got := VerifyV1a(c.keys, c.msgID, c.ts, c.body, c.signature); got != c.want {
			t.Errorf("%s: VerifyV1a = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestParseKeys(t *testing.T) {
	priv := mustPriv(t, specV2aPrivate)
	full := PrivateKeyPrefix + base64.StdEncoding.EncodeToString(priv)
	if k, err := ParsePrivateKey(full); err != nil || !k.Equal(priv) {
		t.Errorf("64-byte private key: %v", err)
	}
	corrupt := append(ed25519.PrivateKey{}, priv...)
	corrupt[63] ^= 1
	urlSafe := PublicKeyPrefix + base64.RawURLEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))
	if _, err := ParsePublicKey(urlSafe); err != nil {
		t.Errorf("url-safe public key: %v", err)
	}
	cases := []struct {
		name  string
		parse func(string) error
		in    string
		want  error
	}{
		{"private with public prefix", privErr, specV2aPublic, ErrKeyPrefix},
		{"public with private prefix", pubErr, specV2aPrivate, ErrKeyPrefix},
		{"whsec secret as a key", privErr, specSecret, ErrKeyPrefix},
		{"bad base64", pubErr, PublicKeyPrefix + "!!!", ErrKeyEncoding},
		{"short public key", pubErr, PublicKeyPrefix + base64.StdEncoding.EncodeToString(make([]byte, 31)), ErrKeyLength},
		{"short private key", privErr, PrivateKeyPrefix + base64.StdEncoding.EncodeToString(make([]byte, 31)), ErrKeyLength},
		{"64-byte key with a wrong public half", privErr, PrivateKeyPrefix + base64.StdEncoding.EncodeToString(corrupt), ErrKeyLength},
	}
	for _, c := range cases {
		if err := c.parse(c.in); !errors.Is(err, c.want) {
			t.Errorf("%s: error = %v, want %v", c.name, err, c.want)
		}
	}
}

func privErr(s string) error { _, err := ParsePrivateKey(s); return err }
func pubErr(s string) error  { _, err := ParsePublicKey(s); return err }

func TestGenerateKeyPair(t *testing.T) {
	privS, pubS := GenerateKeyPair()
	priv, pub := mustPriv(t, privS), mustPub(t, pubS)
	if !pub.Equal(priv.Public()) {
		t.Fatal("generated public key does not match the private key")
	}
	if a, _ := GenerateKeyPair(); a == privS {
		t.Fatal("two generated private keys are equal")
	}
}
