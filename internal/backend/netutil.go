package backend

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var hostnameRe = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9._-]*[A-Za-z0-9])?$`)

// ValidateHost checks that host is a bare hostname or IP literal. It rejects
// userinfo, paths, queries, fragments, ports and whitespace, so a value
// cannot change the authority or path of a request URL.
func ValidateHost(host string) error {
	if host == "" {
		return fmt.Errorf("host is required")
	}
	if len(host) > 253 {
		return fmt.Errorf("host is too long")
	}
	if strings.ContainsAny(host, "@/?#\\ \t\r\n") {
		return fmt.Errorf("invalid host %q", host)
	}
	// Bracketed IPv6 literals are accepted without the brackets too;
	// net.JoinHostPort adds them back.
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
		return nil
	}
	if strings.Contains(host, ":") {
		return fmt.Errorf("invalid host %q", host)
	}
	if !hostnameRe.MatchString(host) {
		return fmt.Errorf("invalid host %q", host)
	}
	return nil
}

// HostIsLoopback reports whether host names the local machine.
func HostIsLoopback(host string) bool {
	h := strings.ToLower(strings.Trim(host, "[]"))
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return true
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// privateHostSuffixes are hostname suffixes that only resolve on a local
// network. .local is mDNS, .home.arpa is the reserved home-network domain,
// and .lan/.internal are common router defaults.
var privateHostSuffixes = []string{".local", ".lan", ".home.arpa", ".internal"}

// HostIsPrivate reports whether host is reachable only from a local network:
// loopback, RFC 1918, CGNAT (100.64.0.0/10, used by Tailscale), IPv4 and
// IPv6 link-local, IPv6 unique local addresses, or a local-only hostname.
func HostIsPrivate(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	h = strings.TrimSuffix(h, ".")
	h = strings.Trim(h, "[]")
	if h == "" {
		return false
	}
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return true
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || isCGNAT(ip)
	}
	for _, suffix := range privateHostSuffixes {
		if strings.HasSuffix(h, suffix) {
			return true
		}
	}
	// A single-label name resolves through the local resolver or mDNS.
	return !strings.Contains(h, ".")
}

// isCGNAT reports whether ip is in the shared address space used by
// carrier-grade NAT and overlay networks such as Tailscale.
func isCGNAT(ip net.IP) bool {
	ip4 := ip.To4()
	return ip4 != nil && ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127
}

// HostIsOnion reports whether host is a Tor onion address.
func HostIsOnion(host string) bool {
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	return strings.HasSuffix(h, ".onion")
}

// RPCBaseURL builds a URL from validated components. The host is placed in
// the URL authority, never interpolated as raw text (H3-14).
func RPCBaseURL(scheme, host string, port int) (string, error) {
	if err := ValidateHost(host); err != nil {
		return "", err
	}
	if port <= 0 || port > 65535 {
		return "", fmt.Errorf("invalid port %d", port)
	}
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("invalid scheme %q", scheme)
	}
	u := url.URL{
		Scheme: scheme,
		Host:   net.JoinHostPort(strings.Trim(host, "[]"), strconv.Itoa(port)),
		Path:   "/",
	}
	return u.String(), nil
}
