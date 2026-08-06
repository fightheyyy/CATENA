package control

import (
	"strings"
	"testing"
)

func TestAPITokenEnvelopeRoundTripAndTamperDetection(t *testing.T) {
	const (
		plaintext = "barena_pat_round_trip_secret"
		tokenID   = "pat-round-trip"
		secret    = "test-only-token-recovery-secret-32-bytes"
	)
	envelope, err := encryptAPIToken(plaintext, tokenID, secret)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(envelope, plaintext) {
		t.Fatal("encrypted envelope contains plaintext")
	}
	recovered, err := decryptAPIToken(envelope, tokenID, secret)
	if err != nil || recovered != plaintext {
		t.Fatalf("unexpected recovery result %q err=%v", recovered, err)
	}
	if _, err := decryptAPIToken(envelope, "pat-other", secret); err == nil {
		t.Fatal("token envelope must be bound to its token ID")
	}
	replacement := "A"
	if strings.HasSuffix(envelope, replacement) {
		replacement = "B"
	}
	tampered := envelope[:len(envelope)-1] + replacement
	if _, err := decryptAPIToken(tampered, tokenID, secret); err == nil {
		t.Fatal("tampered token envelope was accepted")
	}
}

func TestMaskAPIToken(t *testing.T) {
	masked := maskAPIToken("barena_pat_abcdefghijklmnopqrstuvwxyz")
	if masked != "barena_pat_••••••••wxyz" {
		t.Fatalf("unexpected mask %q", masked)
	}
	if fallback := maskAPIToken(""); fallback != "barena_pat_••••••••" {
		t.Fatalf("unexpected legacy mask %q", fallback)
	}
}
