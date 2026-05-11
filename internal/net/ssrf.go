package net

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ValidateOutboundURL parses u and rejects empty, non-http(s), or URLs that
// resolve to a private/loopback/link-local address. When allowInternal is true,
// the address-range check is skipped (operator opt-in for on-prem deployments).
func ValidateOutboundURL(u string, allowInternal bool) error {
	if strings.TrimSpace(u) == "" {
		return errors.New("url is empty")
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return fmt.Errorf("parse url: %w", err)
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && allowInternal) {
		return fmt.Errorf("scheme %q not allowed", parsed.Scheme)
	}
	host := parsed.Hostname()
	if host == "" {
		return errors.New("url has no host")
	}
	if allowInternal {
		return nil
	}
	addrs, err := net.LookupHost(host)
	if err != nil {
		return fmt.Errorf("resolve host: %w", err)
	}
	for _, a := range addrs {
		ip := net.ParseIP(a)
		if ip == nil {
			continue
		}
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return fmt.Errorf("host resolves to disallowed address %s", a)
		}
	}
	return nil
}
