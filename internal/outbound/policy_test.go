package outbound

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
)

func TestDestinationClassification(t *testing.T) {
	p, _ := New("")
	for _, raw := range []string{"127.0.0.1", "::1", "::ffff:127.0.0.1", "10.1.2.3", "192.168.0.1", "169.254.169.254", "100.100.100.200", "168.63.129.16", "0.0.0.0", "::", "fc00::1", "fe80::1", "224.0.0.1", "2001:db8::1", "64:ff9b::7f00:1", "198.18.0.1"} {
		if p.permits(netip.MustParseAddr(raw)) {
			t.Fatal("unsafe address allowed", raw)
		}
	}
	for _, raw := range []string{"8.8.8.8", "2606:4700:4700::1111"} {
		if !p.permits(netip.MustParseAddr(raw)) {
			t.Fatal("public address denied", raw)
		}
	}
	for _, raw := range []string{"file:///etc/passwd", "http://user:secret@example.com/", "http://[fe80::1%25eth0]/"} {
		u, _ := url.Parse(raw)
		if p.CheckURL(u) == nil {
			t.Fatal("unsafe URL allowed")
		}
	}
	allowed, _ := New("127.0.0.1/32,192.168.1.0/24")
	if !allowed.permits(netip.MustParseAddr("127.0.0.1")) || allowed.permits(netip.MustParseAddr("127.0.0.2")) {
		t.Fatal("allowance not narrow")
	}
	if _, err := New("0.0.0.0/0"); err == nil {
		t.Fatal("unrestricted grant accepted")
	}
}

func TestDialValidatesAllDNSAnswersAndUsesPinnedIP(t *testing.T) {
	p, _ := New("")
	calls := 0
	p.lookup = func(context.Context, string, string) ([]netip.Addr, error) {
		calls++
		if calls == 1 {
			return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
		}
		return []netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("127.0.0.1")}, nil
	}
	dialed := 0
	p.dial = func(_ context.Context, _, addr string) (net.Conn, error) {
		dialed++
		if addr != "8.8.8.8:443" {
			t.Fatal("hostname re-resolved", addr)
		}
		a, b := net.Pipe()
		b.Close()
		return a, nil
	}
	conn, err := p.DialContext(context.Background(), "tcp", "fixture.invalid:443")
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	if _, err := p.DialContext(context.Background(), "tcp", "fixture.invalid:443"); !errors.Is(err, ErrDenied) {
		t.Fatal("DNS rebinding/mixed answers allowed", err)
	}
	if dialed != 1 {
		t.Fatal("forbidden DNS answer reached dialer")
	}
}

func TestRealHTTPGrantsRedirectsAndCredentialStripping(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, key := range []string{"Authorization", "Cookie", "X-Api-Key", "Referer"} {
			if r.Header.Get(key) != "" {
				t.Error("credential forwarded", key)
			}
		}
		w.Write([]byte("allowed"))
	}))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/deny":
			http.Redirect(w, r, "http://127.0.0.2/private", 302)
		case "/loop":
			http.Redirect(w, r, "/loop", 302)
		default:
			http.Redirect(w, r, target.URL, 302)
		}
	}))
	defer origin.Close()
	p, _ := New("")
	if _, err := p.Client().Get(origin.URL); err == nil {
		t.Fatal("default reached loopback")
	}
	p, _ = New("127.0.0.1/32")
	r, _ := http.NewRequest("GET", origin.URL, nil)
	r.Header.Set("Authorization", "secret")
	r.Header.Set("X-Api-Key", "secret")
	r.Header.Set("Cookie", "secret")
	resp, err := p.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	for _, suffix := range []string{"/deny", "/loop"} {
		if resp, err := p.Client().Get(origin.URL + suffix); err == nil {
			resp.Body.Close()
			t.Fatal("unsafe redirect allowed", suffix)
		}
	}
	u, _ := url.Parse("http://127.0.0.2/")
	if _, err := p.LookupTracker(u); !errors.Is(err, ErrDenied) {
		t.Fatal("tracker bypass", err)
	}
	if strings.Contains((Failure{Cause: errors.New("http://secret:password/x?key=abc")}).Error(), "password") {
		t.Fatal("sensitive error")
	}
}
