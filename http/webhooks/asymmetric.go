package webhooks

import (
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
)

// Asymmetric signatures in Standard Webhooks use ed25519. The producer holds
// the private key and the receiver trusts the public key, so the receiver can
// tell which producer sent a delivery. A shared HMAC secret can't prove that.
//
// The spec defines two schemes:
//
//   - v2a, current: ed25519 over SHA-256("endpoint_id.msg_id.timestamp.payload").
//     One key pair can serve every endpoint, because the endpoint ID is part of
//     the signed content. The producer shares the endpoint ID with the receiver
//     out of band, and the delivery never carries it.
//   - v1a, deprecated in standard-webhooks PR 372 (2026-11-18): ed25519 over
//     "msg_id.timestamp.payload" itself. A key pair has to be unique per
//     endpoint, or a delivery replays to another endpoint. Specs written
//     before the deprecation still name it, so it stays available here.
//
// A delivery can carry v1, v1a and v2a signatures side by side in the
// webhook-signature header. Each verifier here reads only its own scheme.

// Key prefixes for the serialized ed25519 keys. The rest of each key is base64
// of the raw key bytes.
const (
	PrivateKeyPrefix = "whsk_"
	PublicKeyPrefix  = "whpk_"
)

// Signature identifiers, the part before the comma in a webhook-signature
// entry.
const (
	SchemeV1  = "v1"
	SchemeV1a = "v1a"
	SchemeV2a = "v2a"
)

// Errors returned by the key parsers and the v2a signer.
var (
	ErrKeyPrefix   = errors.New("key must start with whsk_ (private) or whpk_ (public)")
	ErrKeyEncoding = errors.New("portion after the key prefix must be base64-encoded")
	ErrKeyLength   = errors.New("key has the wrong length for ed25519")
	ErrEndpointID  = errors.New("endpoint ID must be non-empty and must not contain '.'")
)

// GenerateKeyPair returns a new ed25519 key pair, serialized as whsk_ and
// whpk_ strings in standard padded base64.
func GenerateKeyPair() (privateKey, publicKey string) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader) // never fails since Go 1.24
	return EncodePrivateKey(priv), EncodePublicKey(pub)
}

// EncodePrivateKey serializes key as whsk_ followed by base64 of its 32-byte
// seed, the form the spec's example key uses.
func EncodePrivateKey(key ed25519.PrivateKey) string {
	return PrivateKeyPrefix + base64.StdEncoding.EncodeToString(key.Seed())
}

// EncodePublicKey serializes key as whpk_ followed by base64 of its 32 bytes.
func EncodePublicKey(key ed25519.PublicKey) string {
	return PublicKeyPrefix + base64.StdEncoding.EncodeToString(key)
}

// ParsePrivateKey reads a whsk_ key. The base64 may hold the 32-byte seed or
// the 64-byte seed-plus-public-key form. In the 64-byte form the public half
// must match the seed, so a corrupted key fails here instead of signing
// deliveries nobody can verify.
func ParsePrivateKey(s string) (ed25519.PrivateKey, error) {
	raw, err := decodeKey(s, PrivateKeyPrefix)
	if err != nil {
		return nil, err
	}
	switch len(raw) {
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(raw), nil
	case ed25519.PrivateKeySize:
		key := ed25519.NewKeyFromSeed(raw[:ed25519.SeedSize])
		if !key.Equal(ed25519.PrivateKey(raw)) {
			return nil, ErrKeyLength
		}
		return key, nil
	}
	return nil, ErrKeyLength
}

// ParsePublicKey reads a whpk_ key.
func ParsePublicKey(s string) (ed25519.PublicKey, error) {
	raw, err := decodeKey(s, PublicKeyPrefix)
	if err != nil {
		return nil, err
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, ErrKeyLength
	}
	return ed25519.PublicKey(raw), nil
}

// SignV2a returns the webhook-signature entry ("v2a,<base64>") for one
// delivery to the endpoint endpointID. It hands the signer the 32-byte SHA-256
// digest of the content, so a KMS or HSM signer that caps the message size
// works. An ed25519.PrivateKey is a crypto.Signer.
//
// It returns ErrEndpointID when endpointID is empty or contains '.', since the
// receiver couldn't split the signed content back apart. Any other error comes
// from the signer.
func SignV2a(signer crypto.Signer, endpointID, msgID, timestamp string, body []byte) (string, error) {
	if !validEndpointID(endpointID) {
		return "", ErrEndpointID
	}
	digest := v2aDigest(endpointID, msgID, timestamp, body)
	sig, err := signer.Sign(nil, digest[:], crypto.Hash(0))
	if err != nil {
		return "", err
	}
	return SchemeV2a + "," + base64.StdEncoding.EncodeToString(sig), nil
}

// VerifyV2a reports whether signature, a webhook-signature header value, holds
// a v2a signature of this delivery under any of keys. endpointID must be the
// one the producer shared when the endpoint was set up, never a value read
// from the request. Entries of other schemes are skipped.
//
// Like Verify, it checks only the signature. Receivers should also check
// TimestampWithinTolerance and dedup on msgID.
func VerifyV2a(keys []ed25519.PublicKey, endpointID, msgID, timestamp string, body []byte, signature string) bool {
	if !validEndpointID(endpointID) {
		return false
	}
	digest := v2aDigest(endpointID, msgID, timestamp, body)
	return verifyAny(keys, SchemeV2a, digest[:], signature)
}

// SignV1a returns the deprecated v1a webhook-signature entry ("v1a,<base64>")
// for one delivery: ed25519 over "msg_id.timestamp.payload". New producers
// should send v2a. The signer receives the whole content, so a KMS signer with
// a message cap fails on large bodies.
func SignV1a(signer crypto.Signer, msgID, timestamp string, body []byte) (string, error) {
	sig, err := signer.Sign(nil, v1Content(msgID, timestamp, body), crypto.Hash(0))
	if err != nil {
		return "", err
	}
	return SchemeV1a + "," + base64.StdEncoding.EncodeToString(sig), nil
}

// VerifyV1a reports whether signature, a webhook-signature header value, holds
// a v1a signature of this delivery under any of keys. Entries of other schemes
// are skipped. v1a carries no endpoint ID, so a receiver that trusts a key
// shared across endpoints accepts deliveries replayed from those endpoints.
func VerifyV1a(keys []ed25519.PublicKey, msgID, timestamp string, body []byte, signature string) bool {
	return verifyAny(keys, SchemeV1a, v1Content(msgID, timestamp, body), signature)
}

func v1Content(msgID, timestamp string, body []byte) []byte {
	content := make([]byte, 0, len(msgID)+len(timestamp)+len(body)+2)
	content = append(content, msgID...)
	content = append(content, '.')
	content = append(content, timestamp...)
	content = append(content, '.')
	return append(content, body...)
}

func v2aDigest(endpointID, msgID, timestamp string, body []byte) [sha256.Size]byte {
	h := sha256.New()
	h.Write([]byte(endpointID))
	h.Write([]byte("."))
	h.Write(v1Content(msgID, timestamp, body))
	var out [sha256.Size]byte
	h.Sum(out[:0])
	return out
}

func validEndpointID(id string) bool {
	return id != "" && !strings.Contains(id, ".")
}

func verifyAny(keys []ed25519.PublicKey, scheme string, message []byte, signature string) bool {
	prefix := scheme + ","
	for _, entry := range strings.Fields(signature) {
		encoded, ok := strings.CutPrefix(entry, prefix)
		if !ok {
			continue
		}
		sig, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(sig) != ed25519.SignatureSize {
			continue
		}
		for _, key := range keys {
			if len(key) == ed25519.PublicKeySize && ed25519.Verify(key, message, sig) {
				return true
			}
		}
	}
	return false
}

func decodeKey(s, prefix string) ([]byte, error) {
	encoded, ok := strings.CutPrefix(s, prefix)
	if !ok {
		return nil, ErrKeyPrefix
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		raw, err = base64.RawURLEncoding.DecodeString(encoded)
		if err != nil {
			return nil, ErrKeyEncoding
		}
	}
	return raw, nil
}
