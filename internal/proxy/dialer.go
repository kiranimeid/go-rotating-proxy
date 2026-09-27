package proxy

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/kiranimeid/go-rotating-proxy/internal/config"
	socks5 "golang.org/x/net/proxy"
)


type Dialer interface {
	Dial(network, address string) (net.Conn, error)
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
	Transport() *http.Transport
	String() string
}

type socksDialer struct {
	proxy     config.Proxy
	underlying socks5.Dialer
	transport *http.Transport
}

func newSocksDialer(p config.Proxy) (*socksDialer, error) {
	var auth *socks5.Auth
	if p.User != "" || p.Pass != "" {
		auth = &socks5.Auth{User: p.User, Password: p.Pass}
	}

	direct := socks5.Direct
	d, err := socks5.SOCKS5("tcp", p.Addr(), auth, direct)
	if err != nil {
		return nil, err
	}

	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if ctxDialer, ok := d.(socks5.ContextDialer); ok {
				return ctxDialer.DialContext(ctx, network, addr)
			}
			return d.Dial(network, addr)
		},
		DisableKeepAlives: true,
	}

	return &socksDialer{
		proxy:      p,
		underlying: d,
		transport:  tr,
	}, nil
}

func (s *socksDialer) Dial(network, address string) (net.Conn, error) {
	return s.underlying.Dial(network, address)
}

func (s *socksDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if cd, ok := s.underlying.(socks5.ContextDialer); ok {
		return cd.DialContext(ctx, network, address)
	}
	return s.underlying.Dial(network, address)
}

func (s *socksDialer) Transport() *http.Transport {
	return s.transport
}

func (s *socksDialer) String() string {
	return fmt.Sprintf("socks5://%s", s.proxy.Addr())
}

type httpDialer struct {
	proxy     config.Proxy
	transport *http.Transport
}

func newHTTPDialer(p config.Proxy) *httpDialer {
	proxyURL := &url.URL{
		Scheme: "http",
		Host:   p.Addr(),
	}
	if p.User != "" || p.Pass != "" {
		proxyURL.User = url.UserPassword(p.User, p.Pass)
	}

	tr := &http.Transport{
		Proxy:             http.ProxyURL(proxyURL),
		DisableKeepAlives: true,
	}

	return &httpDialer{
		proxy:     p,
		transport: tr,
	}
}

func (h *httpDialer) Dial(network, address string) (net.Conn, error) {
	return h.DialContext(context.Background(), network, address)
}

func (h *httpDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", h.proxy.Addr())
	if err != nil {
		return nil, fmt.Errorf("connect to upstream proxy failed: %w", err)
	}

	req := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", address, address)
	if h.proxy.User != "" || h.proxy.Pass != "" {
		auth := base64.StdEncoding.EncodeToString([]byte(h.proxy.User + ":" + h.proxy.Pass))
		req += fmt.Sprintf("Proxy-Authorization: Basic %s\r\n", auth)
	}
	req += "\r\n"

	if _, err := conn.Write([]byte(req)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("write CONNECT request failed: %w", err)
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("read CONNECT response failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		conn.Close()
		return nil, fmt.Errorf("upstream proxy CONNECT returned %s", resp.Status)
	}

	
	if br.Buffered() > 0 {
		conn.Close()
		return nil, errors.New("unexpected buffered data after CONNECT")
	}

	return conn, nil
}

func (h *httpDialer) Transport() *http.Transport {
	return h.transport
}

func (h *httpDialer) String() string {
	return fmt.Sprintf("http://%s", h.proxy.Addr())
}


func NewDialer(p config.Proxy) (Dialer, error) {
	switch p.Scheme {
	case "socks5", "socks":
		return newSocksDialer(p)
	default: 
		return newHTTPDialer(p), nil
	}
}


type DirectDialer struct {
	dialer    net.Dialer
	transport *http.Transport
}


func NewDirectDialer() *DirectDialer {
	return &DirectDialer{
		dialer: net.Dialer{Timeout: 10 * time.Second},
		transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}
}

func (d *DirectDialer) Dial(network, address string) (net.Conn, error) {
	return d.dialer.Dial(network, address)
}

func (d *DirectDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return d.dialer.DialContext(ctx, network, address)
}

func (d *DirectDialer) Transport() *http.Transport {
	return d.transport
}

func (d *DirectDialer) String() string {
	return "direct"
}
