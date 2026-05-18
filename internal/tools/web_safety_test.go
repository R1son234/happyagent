package tools

import (
	"context"
	"strings"
	"testing"

	"happyagent/internal/config"
)

func TestWebURLPolicyRejectsInternalURLsByDefault(t *testing.T) {
	policy := newWebURLPolicy(config.WebConfig{})
	urls := []string{
		"http://localhost:8080",
		"http://127.0.0.1:8080",
		"http://[::1]:8080",
		"http://169.254.169.254/latest/meta-data",
		"http://10.0.0.1",
	}

	for _, rawURL := range urls {
		if _, err := policy.validatePublicHTTPURL(context.Background(), rawURL); err == nil {
			t.Fatalf("validatePublicHTTPURL(%q) expected error", rawURL)
		}
	}
}

func TestWebURLPolicyAllowsPrivateNetworksWhenConfigured(t *testing.T) {
	policy := newWebURLPolicy(config.WebConfig{AllowPrivateNetworks: true})

	if _, err := policy.validatePublicHTTPURL(context.Background(), "http://127.0.0.1:8080"); err != nil {
		t.Fatalf("validatePublicHTTPURL() error = %v", err)
	}
}

func TestWebURLPolicyRejectsBlockedDomains(t *testing.T) {
	policy := newWebURLPolicy(config.WebConfig{
		BlockedDomains: []string{"example.com", "*.blocked.test"},
	})

	for _, host := range []string{"example.com", "sub.example.com", "x.blocked.test"} {
		err := policy.validateHost(host)
		if err == nil || !strings.Contains(err.Error(), "blocked") {
			t.Fatalf("validateHost(%q) error = %v", host, err)
		}
	}
	if err := policy.validateHost("blocked.test"); err != nil {
		t.Fatalf("wildcard rule should not block bare domain: %v", err)
	}
}

func TestWebURLPolicyRejectsSecretLikeURL(t *testing.T) {
	policy := newWebURLPolicy(config.WebConfig{})

	_, err := policy.validatePublicHTTPURL(context.Background(), "https://example.com/?api_key=secret")
	if err == nil || !strings.Contains(err.Error(), "secret") {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = policy.validatePublicHTTPURL(context.Background(), "https://example.com/?q=api%5Fkey%3Dsecret")
	if err == nil || !strings.Contains(err.Error(), "secret") {
		t.Fatalf("unexpected percent-decoded error: %v", err)
	}
}

func TestWebURLPolicyAllowsPublicIPLiteral(t *testing.T) {
	policy := newWebURLPolicy(config.WebConfig{})

	if _, err := policy.validatePublicHTTPURL(context.Background(), "http://93.184.216.34/article"); err != nil {
		t.Fatalf("validatePublicHTTPURL() error = %v", err)
	}
}

func TestWebURLPolicyResolvedAddressRejectsPrivateIP(t *testing.T) {
	policy := newWebURLPolicy(config.WebConfig{})

	err := policy.validateResolvedAddress(context.Background(), "127.0.0.1:80")
	if err == nil || !strings.Contains(err.Error(), "private") && !strings.Contains(err.Error(), "localhost") {
		t.Fatalf("unexpected error: %v", err)
	}
}
