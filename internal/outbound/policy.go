// Package outbound supplies the application's HTTP destination boundary. It
// controls HTTP, not peer sockets, DHT, UDP discovery or anonymity.
package outbound

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

var ErrDenied = errors.New("outbound destination is not allowed")

type Policy struct {
	allowed []netip.Prefix
	lookup  func(context.Context, string, string) ([]netip.Addr, error)
	dial    func(context.Context, string, string) (net.Conn, error)
}

func New(allowCIDRs string) (*Policy, error) {
	d := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	p := &Policy{lookup: net.DefaultResolver.LookupNetIP, dial: d.DialContext}
	for _, value := range strings.Split(allowCIDRs, ",") {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return nil, errors.New("outbound allowances must be IP CIDRs")
		}
		if prefix.Bits() == 0 {
			return nil, errors.New("unrestricted outbound allowances are not supported")
		}
		p.allowed = append(p.allowed, prefix.Masked())
	}
	return p, nil
}

var special = func() []netip.Prefix {
	var result []netip.Prefix
	for _, s := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24", "168.63.129.16/32", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001::/32", "2001:db8::/32", "2002::/16", "64:ff9b::/96", "64:ff9b:1::/48"} {
		result = append(result, netip.MustParsePrefix(s))
	}
	return result
}()

func (p *Policy) permits(ip netip.Addr) bool {
	if !ip.IsValid() || ip.Zone() != "" {
		return false
	}
	ip = ip.Unmap()
	for _, prefix := range p.allowed {
		if prefix.Contains(ip) {
			return true
		}
	}
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, prefix := range special {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}

func (p *Policy) CheckURL(u *url.URL) error {
	if u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Hostname() == "" || strings.Contains(u.Hostname(), "%") {
		return ErrDenied
	}
	if ip, err := netip.ParseAddr(u.Hostname()); err == nil && !p.permits(ip) {
		return ErrDenied
	}
	return nil
}

func (p *Policy) resolve(ctx context.Context, host string) ([]netip.Addr, error) {
	var ips []netip.Addr
	if ip, err := netip.ParseAddr(host); err == nil {
		ips = []netip.Addr{ip}
	} else {
		var err error
		ips, err = p.lookup(ctx, "ip", host)
		if err != nil {
			return nil, ErrDenied
		}
	}
	if len(ips) == 0 {
		return nil, ErrDenied
	}
	for _, ip := range ips {
		if !p.permits(ip) {
			return nil, ErrDenied
		}
	}
	return ips, nil
}

func (p *Policy) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, ErrDenied
	}
	ips, err := p.resolve(ctx, host)
	if err != nil {
		return nil, err
	}
	var last error
	for _, ip := range ips {
		conn, err := p.dial(ctx, network, net.JoinHostPort(ip.Unmap().String(), port))
		if err == nil {
			return conn, nil
		}
		last = err
	}
	return nil, last
}

// Proxy validates every HTTP request, including library-owned redirect hops.
// Returning nil disables ambient HTTP_PROXY settings that could bypass dialing.
func (p *Policy) Proxy(r *http.Request) (*url.URL, error) { return nil, p.CheckURL(r.URL) }

func (p *Policy) LookupTracker(u *url.URL) ([]net.IP, error) {
	// UDP trackers use a separate transport, but still honor destination grants.
	if u == nil || u.User != nil || u.Hostname() == "" {
		return nil, ErrDenied
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ips, err := p.resolve(ctx, u.Hostname())
	if err != nil {
		return nil, err
	}
	result := make([]net.IP, len(ips))
	for i, ip := range ips {
		result[i] = net.IP(ip.AsSlice())
	}
	return result, nil
}

func (p *Policy) Transport() http.RoundTripper {
	return &http.Transport{Proxy: p.Proxy, DialContext: p.DialContext, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 15 * time.Second, IdleConnTimeout: 30 * time.Second, MaxConnsPerHost: 10, MaxIdleConns: 20}
}

func (p *Policy) Client() *http.Client {
	return &http.Client{Transport: p.Transport(), Timeout: 15 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many outbound redirects")
		}
		if len(via) > 0 && (!strings.EqualFold(via[len(via)-1].URL.Host, r.URL.Host) || via[len(via)-1].URL.Scheme != r.URL.Scheme) {
			safe := http.Header{}
			for _, key := range []string{"Accept", "Accept-Encoding", "User-Agent"} {
				if value := r.Header.Get(key); value != "" {
					safe.Set(key, value)
				}
			}
			r.Header = safe
		}
		return p.CheckURL(r.URL)
	}}
}

// Failure preserves cancellation checks without exposing URL userinfo, passkeys,
// query parameters or provider headers through errors.
type Failure struct{ Cause error }

func (e Failure) Error() string { return "remote request failed" }
func (e Failure) Unwrap() error { return e.Cause }
