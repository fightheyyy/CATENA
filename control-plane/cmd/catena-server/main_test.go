package main

import (
	"net"
	"testing"
)

func TestValidateBinding(t *testing.T) {
	t.Parallel()
	loopback := net.ParseIP("127.0.0.1")
	remote := net.ParseIP("0.0.0.0")
	secret := "test-only-internal-gateway-secret-32-bytes"

	if err := validateBinding(loopback, false, false, "", "", "", ""); err != nil {
		t.Fatalf("loopback rejected: %v", err)
	}
	if err := validateBinding(remote, false, true, secret, "", "", ""); err == nil {
		t.Fatal("remote binding without explicit allow was accepted")
	}
	if err := validateBinding(remote, true, true, "short", "", "", ""); err == nil {
		t.Fatal("internal binding with a short gateway secret was accepted")
	}
	if err := validateBinding(remote, true, true, secret, "", "", ""); err != nil {
		t.Fatalf("private internal binding rejected: %v", err)
	}
	if err := validateBinding(remote, true, false, "", "client", "secret", "http://example.com"); err == nil {
		t.Fatal("public binding without HTTPS OAuth callback was accepted")
	}
	if err := validateBinding(remote, true, false, "", "client", "secret", "https://example.com/callback"); err != nil {
		t.Fatalf("valid public binding rejected: %v", err)
	}
}
