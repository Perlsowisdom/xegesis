package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Perlsowisdom/xegesis/internal/update/updatesig"
)

func TestKeygenSignVerify(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "key.txt")
	var out bytes.Buffer
	if err := run([]string{"keygen", "-out", keyFile}, &out, func(string) string { return "" }); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	pub, err := updatesig.ParsePublicKey(lines[len(lines)-1])
	if err != nil {
		t.Fatalf("keygen did not print the public key last: %v", err)
	}
	if err := run([]string{"keygen", "-out", keyFile}, &out, nil); err == nil {
		t.Error("keygen overwrote an existing key file")
	}

	installer := filepath.Join(dir, "setup.exe")
	if err := os.WriteFile(installer, []byte("installer"), 0o600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := run([]string{"hash", installer}, &out, nil); err != nil {
		t.Fatal(err)
	}
	hash := strings.TrimSpace(out.String())

	key, _ := os.ReadFile(keyFile)
	out.Reset()
	// The key comes from the environment variable, as in CI.
	if err := run([]string{"sign", "-version", "1.2.3", installer}, &out, func(string) string { return string(key) }); err != nil {
		t.Fatal(err)
	}
	if err := updatesig.Verify(pub, "1.2.3", hash, strings.TrimSpace(out.String())); err != nil {
		t.Fatalf("signature does not verify: %v", err)
	}
	sig := strings.TrimSpace(out.String())
	pubFile := filepath.Join(dir, "pubkey.txt")
	if err := os.WriteFile(pubFile, []byte("# test key"+string(rune(10))+lines[len(lines)-1]+string(rune(10))), 0o600); err != nil {
		t.Fatal(err)
	}
	var vout bytes.Buffer
	if err := run([]string{"verify", "-version", "1.2.3", "-pub", pubFile, installer, sig}, &vout, nil); err != nil || !strings.Contains(vout.String(), "OK") {
		t.Fatalf("verify: %v %q", err, vout.String())
	}
	if err := run([]string{"verify", "-version", "1.2.4", "-pub", pubFile, installer, sig}, &vout, nil); err == nil {
		t.Error("verify accepted the wrong version")
	}
	if err := run([]string{"sign", "-version", "1.2.3", installer}, &out, func(string) string { return "" }); err == nil {
		t.Error("signed without a key")
	}
	if err := run([]string{"sign", installer}, &out, func(string) string { return string(key) }); err == nil {
		t.Error("signed without a version")
	}
}
