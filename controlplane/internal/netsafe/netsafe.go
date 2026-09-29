// Package netsafe validates URLs and resolved addresses against SSRF policy.
package netsafe

import (
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
)

// ErrBlocked is returned when a URL or address violates policy.
var ErrBlocked = fmt.Errorf("address blocked by SSRF policy")

// ValidateURL checks scheme and host before DNS resolution.
func ValidateURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("%w: empty url", ErrBlocked)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBlocked, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("%w: scheme %q not allowed", ErrBlocked, u.Scheme)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("%w: missing host", ErrBlocked)
	}
	if u.User != nil {
		return nil, fmt.Errorf("%w: userinfo not allowed", ErrBlocked)
	}
	host := u.Hostname()
	if host == "" {
		return nil, fmt.Errorf("%w: missing hostname", ErrBlocked)
	}
	if isBlockedHostname(host) {
		return nil, fmt.Errorf("%w: host %q not allowed", ErrBlocked, host)
	}
	// Literal IP in hostname.
	if ip, err := netip.ParseAddr(host); err == nil {
		if IsBlockedIP(ip) {
			return nil, fmt.Errorf("%w: address %s not allowed", ErrBlocked, ip)
		}
	}
	return u, nil
}

// ValidateResolved checks every address returned by DNS for a host.
func ValidateResolved(host string, addrs []net.IP) error {
	if len(addrs) == 0 {
		return fmt.Errorf("%w: no addresses for %q", ErrBlocked, host)
	}
	for _, addr := range addrs {
		ip, ok := netip.AddrFromSlice(addr)
		if !ok {
			return fmt.Errorf("%w: invalid address %v", ErrBlocked, addr)
		}
		ip = ip.Unmap()
		if IsBlockedIP(ip) {
			return fmt.Errorf("%w: address %s not allowed", ErrBlocked, ip)
		}
	}
	return nil
}

// IsBlockedIP reports whether ip is loopback, private, link-local, or metadata.
func IsBlockedIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() || ip.IsInterfaceLocalMulticast() {
		return true
	}
	// Cloud metadata commonly used SSRF target.
	if ip.String() == "169.254.169.254" {
		return true
	}
	// IPv6 unique local (fc00::/7) covered by IsPrivate in Go 1.22+.
	return false
}

func isBlockedHostname(host string) bool {
	h := strings.ToLower(host)
	switch h {
	case "localhost", "localhost.", "metadata", "metadata.google.internal":
		return true
	}
	if strings.HasSuffix(h, ".localhost") || strings.HasSuffix(h, ".local") {
		return true
	}
	return false
}
