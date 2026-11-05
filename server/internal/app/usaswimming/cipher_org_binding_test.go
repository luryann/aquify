package usaswimming

import "testing"

func TestCipherBindsSecretsToTheirOrg(t *testing.T) {
	c := NewCipher("k")
	enc, err := c.EncryptFor("org-a", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := c.DecryptFor("org-a", enc); err != nil || got != "secret" {
		t.Fatalf("own org: %q %v", got, err)
	}
	if _, err := c.DecryptFor("org-b", enc); err == nil {
		t.Fatal("a ciphertext copied into another org's row must not decrypt")
	}
	legacy, _ := c.Encrypt("old")
	if got, err := c.DecryptFor("org-a", legacy); err != nil || got != "old" {
		t.Fatalf("legacy unbound values stay readable: %q %v", got, err)
	}
}
