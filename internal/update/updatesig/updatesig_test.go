package updatesig

import (
	"strings"
	"testing"
)

func TestSignVerify(t *testing.T) {
	pubB64, priv, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	pub, err := ParsePublicKey("# a comment\n\n" + pubB64 + "\n")
	if err != nil {
		t.Fatal(err)
	}
	h, _ := Sum(strings.NewReader("installer bytes"))
	sig, err := Sign(priv, "1.2.3", h)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(pub, "1.2.3", h, sig); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	other, _ := Sum(strings.NewReader("other bytes"))
	for name, err := range map[string]error{
		"wrong hash":    Verify(pub, "1.2.3", other, sig),
		"wrong version": Verify(pub, "1.2.4", h, sig),
		"bad signature": Verify(pub, "1.2.3", h, "not base64!"),
		"short sig":     Verify(pub, "1.2.3", h, "AAAA"),
		"bad hash":      Verify(pub, "1.2.3", "zz", sig),
	} {
		if err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	otherPub, otherPriv, _ := GenerateKey()
	forged, _ := Sign(otherPriv, "1.2.3", h)
	pk, _ := ParsePublicKey(otherPub)
	if err := Verify(pub, "1.2.3", h, forged); err == nil {
		t.Error("a signature from another key was accepted")
	}
	if err := Verify(pk, "1.2.3", h, forged); err != nil {
		t.Errorf("the other key's own signature rejected: %v", err)
	}
}

func TestBadKeys(t *testing.T) {
	if _, err := ParsePublicKey("# only a comment"); err == nil {
		t.Error("no key accepted")
	}
	if _, err := ParsePublicKey("AAAA"); err == nil {
		t.Error("short key accepted")
	}
	if _, err := Sign("AAAA", "1.0.0", strings.Repeat("0", 64)); err == nil {
		t.Error("short private key accepted")
	}
}
