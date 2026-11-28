package repo

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func legacySHA256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func TestHexSHA256KnownVectors(t *testing.T) {
	cases := map[string]string{
		"":    "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		"abc": "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
	}
	for in, want := range cases {
		if got := hexSHA256(in); got != want {
			t.Fatalf("hexSHA256(%q) = %s, want %s", in, got, want)
		}
	}
}
