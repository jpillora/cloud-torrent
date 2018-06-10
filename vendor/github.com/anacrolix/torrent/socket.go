package torrent

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/proxy"

	"github.com/anacrolix/missinggo"
)

type dialer interface {
	dial(_ context.Context, addr string) (net.Conn, error)
}

type socket interface {
	net.Listener
	dialer
}

func getProxyDialer(proxyURL string) (proxy.Dialer, error) {
	fixedURL, err := url.Parse(proxyURL)
	if err != nil {
		return nil, err
	}

	return proxy.FromURL(fixedURL, proxy.Direct)
}

func listen(network, addr, proxyURL string) (socket, error) {
	if isTcpNetwork(network) {
		return listenTcp(network, addr, proxyURL)
	} else if isUtpNetwork(network) {
		return listenUtp(network, addr, proxyURL)
	} else {
		panic(fmt.Sprintf("unknown network %q", network))
	}
}

func isTcpNetwork(s string) bool {
	return strings.Contains(s, "tcp")
}

func isUtpNetwork(s string) bool {
	return strings.Contains(s, "utp") || strings.Contains(s, "udp")
}

func listenTcp(network, address, proxyURL string) (s socket, err error) {
	l, err := net.Listen(network, address)
	if err != nil {
		return
	}

	// If we don't need the proxy - then we should return default net.Dialer,
	// otherwise, let's try to parse the proxyURL and return proxy.Dialer
	if len(proxyURL) != 0 {
		if dialer, err := getProxyDialer(proxyURL); err == nil {
			return tcpSocket{l, dialer}, nil
		}
	}

	return tcpSocket{l, nil}, nil
}

type tcpSocket struct {
	net.Listener
	d proxy.Dialer
}

func (me tcpSocket) dial(ctx context.Context, addr string) (net.Conn, error) {
	if me.d != nil {
		return me.d.Dial(me.Addr().Network(), addr)
	}

	return net.Dial(me.Addr().Network(), addr)
}

func setPort(addr string, port int) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		panic(err)
	}
	return net.JoinHostPort(host, strconv.FormatInt(int64(port), 10))
}

func listenAll(networks []string, getHost func(string) string, port int, proxyURL string) ([]socket, error) {
	if len(networks) == 0 {
		return nil, nil
	}
	var nahs []networkAndHost
	for _, n := range networks {
		nahs = append(nahs, networkAndHost{n, getHost(n)})
	}
	for {
		ss, retry, err := listenAllRetry(nahs, port, proxyURL)
		if !retry {
			return ss, err
		}
	}
}

type networkAndHost struct {
	Network string
	Host    string
}

func listenAllRetry(nahs []networkAndHost, port int, proxyURL string) (ss []socket, retry bool, err error) {
	ss = make([]socket, 1, len(nahs))
	portStr := strconv.FormatInt(int64(port), 10)
	ss[0], err = listen(nahs[0].Network, net.JoinHostPort(nahs[0].Host, portStr), proxyURL)
	if err != nil {
		return nil, false, fmt.Errorf("first listen: %s", err)
	}
	defer func() {
		if err != nil || retry {
			for _, s := range ss {
				s.Close()
			}
			ss = nil
		}
	}()
	portStr = strconv.FormatInt(int64(missinggo.AddrPort(ss[0].Addr())), 10)
	for _, nah := range nahs[1:] {
		s, err := listen(nah.Network, net.JoinHostPort(nah.Host, portStr), proxyURL)
		if err != nil {
			return ss,
				missinggo.IsAddrInUse(err) && port == 0,
				fmt.Errorf("subsequent listen: %s", err)
		}
		ss = append(ss, s)
	}
	return
}

func listenUtp(network, addr, proxyURL string) (s socket, err error) {
	us, err := NewUtpSocket(network, addr)
	if err != nil {
		return
	}

	// If we don't need the proxy - then we should return default net.Dialer,
	// otherwise, let's try to parse the proxyURL and return proxy.Dialer
	if len(proxyURL) != 0 {
		if dialer, err := getProxyDialer(proxyURL); err == nil {
			return utpSocketSocket{us, network, dialer}, nil
		}
	}

	return utpSocketSocket{us, network, nil}, nil
}

type utpSocketSocket struct {
	utpSocket
	network string
	d       proxy.Dialer
}

func (me utpSocketSocket) dial(ctx context.Context, addr string) (net.Conn, error) {
	if me.d != nil {
		return me.d.Dial(me.network, addr)
	}

	return me.utpSocket.DialContext(ctx, me.network, addr)
}
