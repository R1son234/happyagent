package tools

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"

	"happyagent/internal/config"
)

var secretLikeURLPattern = regexp.MustCompile(`(?i)(api[_-]?key|access[_-]?token|auth[_-]?token|secret|password|passwd|token)=|sk-[A-Za-z0-9_-]{12,}`)

type webURLPolicy struct {
	cfg config.WebConfig // cfg contains the runtime web safety settings.
}

func newWebURLPolicy(cfg config.WebConfig) webURLPolicy {
	return webURLPolicy{cfg: cfg}
}

func (p webURLPolicy) validatePublicHTTPURL(ctx context.Context, raw string) (*url.URL, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("url must not be empty")
	}
	decoded, _ := url.QueryUnescape(trimmed)
	if secretLikeURLPattern.MatchString(trimmed) || secretLikeURLPattern.MatchString(decoded) {
		return nil, fmt.Errorf("url contains a value that looks like a secret")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("url scheme must be http or https")
	}
	if parsed.Hostname() == "" {
		return nil, fmt.Errorf("url host must not be empty")
	}
	if err := p.validateHost(parsed.Hostname()); err != nil {
		return nil, err
	}
	if err := p.validateResolvedHost(ctx, parsed.Hostname()); err != nil {
		return nil, err
	}
	return parsed, nil
}

func (p webURLPolicy) validateHost(host string) error {
	normalized := normalizeWebHost(host)
	if normalized == "" {
		return fmt.Errorf("url host must not be empty")
	}
	if isBlockedWebDomain(normalized, p.cfg.BlockedDomains) {
		return fmt.Errorf("url host %q is blocked by web.blocked_domains", normalized)
	}
	if p.cfg.AllowPrivateNetworks {
		return nil
	}
	if normalized == "localhost" {
		return fmt.Errorf("url targets localhost, which is blocked by default")
	}
	if ip := net.ParseIP(normalized); ip != nil && isPrivateOrInternalIP(ip) {
		return fmt.Errorf("url targets a private or internal network address")
	}
	return nil
}

func (p webURLPolicy) validateResolvedAddress(ctx context.Context, address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	return p.validateResolvedHost(ctx, host)
}

func (p webURLPolicy) validateResolvedHost(ctx context.Context, host string) error {
	if err := p.validateHost(host); err != nil {
		return err
	}
	if p.cfg.AllowPrivateNetworks {
		return nil
	}
	if net.ParseIP(host) != nil {
		return nil
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("resolve url host %q: %w", host, err)
	}
	for _, addr := range addrs {
		if isPrivateOrInternalIP(addr.IP) {
			return fmt.Errorf("url host %q resolves to a private or internal network address", host)
		}
	}
	return nil
}

func normalizeWebHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}

func isBlockedWebDomain(host string, rules []string) bool {
	host = strings.TrimPrefix(normalizeWebHost(host), "www.")
	for _, rule := range rules {
		rule = normalizeWebHost(rule)
		if rule == "" {
			continue
		}
		if strings.HasPrefix(rule, "http://") || strings.HasPrefix(rule, "https://") {
			if parsed, err := url.Parse(rule); err == nil {
				rule = normalizeWebHost(parsed.Hostname())
			}
		}
		rule = strings.TrimPrefix(rule, "www.")
		if strings.HasPrefix(rule, "*.") {
			suffix := strings.TrimPrefix(rule, "*.")
			if host != suffix && strings.HasSuffix(host, "."+suffix) {
				return true
			}
			continue
		}
		if host == rule || strings.HasSuffix(host, "."+rule) {
			return true
		}
	}
	return false
}

func isPrivateOrInternalIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	if v4 := ip.To4(); v4 != nil && v4[0] == 169 && v4[1] == 254 && v4[2] == 169 && v4[3] == 254 {
		return true
	}
	return false
}
