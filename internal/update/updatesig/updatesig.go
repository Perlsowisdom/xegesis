// Package updatesig signs and verifies Xegesis updates. The installer is not code-signed, so every
// update carries an Ed25519 signature that the app checks against the public key built into it.
//
// The signature covers the installer's SHA-256 and the version it is released as, so an old
// installer cannot be passed off as a new release.
package updatesig

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Message is the exact text that is signed: a fixed label, the version and the lower-case hex
// SHA-256 of the installer.
func Message(version, sha256hex string) []byte {
	return []byte("xegesis-update-v1\n" + version + "\n" + strings.ToLower(sha256hex) + "\n")
}

// Sum returns the lower-case hex SHA-256 of r's contents.
func Sum(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// GenerateKey makes a key pair, both base64-encoded: the public key (32 bytes) to commit in
// internal/update/pubkey.txt, and the private key (64 bytes) to keep secret.
func GenerateKey() (pub, priv string, err error) {
	p, k, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(p), base64.StdEncoding.EncodeToString(k), nil
}

// Sign signs an installer's hash for a version with a base64 private key and returns the base64
// signature.
func Sign(privB64, version, sha256hex string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(privB64))
	if err != nil || len(raw) != ed25519.PrivateKeySize {
		return "", errors.New("the private key is not a valid base64 Ed25519 key")
	}
	if err := checkHash(sha256hex); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ed25519.Sign(ed25519.PrivateKey(raw), Message(version, sha256hex))), nil
}

// ParsePublicKey reads a base64 public key. Blank lines and lines starting with # are ignored, so
// pubkey.txt can carry a comment.
func ParsePublicKey(text string) (ed25519.PublicKey, error) {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(line)
		if err != nil || len(raw) != ed25519.PublicKeySize {
			return nil, errors.New("the public key is not a valid base64 Ed25519 key")
		}
		return ed25519.PublicKey(raw), nil
	}
	return nil, errors.New("no public key found")
}

// Verify checks a base64 signature over version and hash.
func Verify(pub ed25519.PublicKey, version, sha256hex, sigB64 string) error {
	if len(pub) != ed25519.PublicKeySize {
		return errors.New("no public key")
	}
	if err := checkHash(sha256hex); err != nil {
		return err
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(sigB64))
	if err != nil || len(sig) != ed25519.SignatureSize {
		return errors.New("the signature is malformed")
	}
	if !ed25519.Verify(pub, Message(version, sha256hex), sig) {
		return errors.New("the signature does not match")
	}
	return nil
}

func checkHash(h string) error {
	if raw, err := hex.DecodeString(h); err != nil || len(raw) != sha256.Size {
		return fmt.Errorf("%q is not a SHA-256 hash", h)
	}
	return nil
}
