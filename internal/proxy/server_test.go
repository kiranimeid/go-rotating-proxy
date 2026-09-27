package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/kiranimeid/go-rotating-proxy/internal/config"
)

func TestServerRoundRobinAndStaticPorts(t *testing.T) {

	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Target-Ack", "true")
		_, _ = w.Write([]byte("target-payload"))
	}))
	defer targetServer.Close()


	upstream1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Upstream-ID", "upstream-1")
		_, _ = w.Write([]byte("from-upstream-1"))
	}))
	defer upstream1.Close()

	upstream2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Upstream-ID", "upstream-2")
		_, _ = w.Write([]byte("from-upstream-2"))
	}))
	defer upstream2.Close()

	u1URL, _ := url.Parse(upstream1.URL)
	u1Host, u1PortStr, _ := net.SplitHostPort(u1URL.Host)
	u1Port, _ := strconv.Atoi(u1PortStr)

	u2URL, _ := url.Parse(upstream2.URL)
	u2Host, u2PortStr, _ := net.SplitHostPort(u2URL.Host)
	u2Port, _ := strconv.Atoi(u2PortStr)


	lRot, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	rotPort := lRot.Addr().(*net.TCPAddr).Port
	lRot.Close()

	lStat1, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	statPort1 := lStat1.Addr().(*net.TCPAddr).Port
	lStat1.Close()

	lStat2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	statPort2 := lStat2.Addr().(*net.TCPAddr).Port
	lStat2.Close()

	proxies := []config.Proxy{
		{
			Host:       u1Host,
			Port:       u1Port,
			Scheme:     "http",
			StaticPort: statPort1,
		},
		{
			Host:       u2Host,
			Port:       u2Port,
			Scheme:     "http",
			StaticPort: statPort2,
		},
	}

	server, err := NewServer(Config{
		ListenAddr: fmt.Sprintf("127.0.0.1:%d", rotPort),
		Whitelist:  config.NewIPWhitelist(),
		Proxies:    proxies,
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = server.Start(ctx)
	}()


	time.Sleep(100 * time.Millisecond)


	rotProxyURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", rotPort))
	rotClient := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(rotProxyURL),
		},
		Timeout: 5 * time.Second,
	}

	resp1, err := rotClient.Get(targetServer.URL)
	if err != nil {
		t.Fatalf("req 1 through rotating proxy failed: %v", err)
	}
	body1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()
	id1 := resp1.Header.Get("X-Upstream-ID")

	resp2, err := rotClient.Get(targetServer.URL)
	if err != nil {
		t.Fatalf("req 2 through rotating proxy failed: %v", err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	id2 := resp2.Header.Get("X-Upstream-ID")

	if id1 == id2 {
		t.Errorf("expected different upstreams for consecutive requests, got %s and %s", id1, id2)
	}
	if string(body1) == string(body2) {
		t.Errorf("expected different response bodies from rotated upstreams, got %s and %s", body1, body2)
	}


	stat1URL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", statPort1))
	stat1Client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(stat1URL),
		},
		Timeout: 5 * time.Second,
	}
	respStat1, err := stat1Client.Get(targetServer.URL)
	if err != nil {
		t.Fatalf("req through static port 1 failed: %v", err)
	}
	defer respStat1.Body.Close()
	if respStat1.Header.Get("X-Upstream-ID") != "upstream-1" {
		t.Errorf("expected upstream-1, got %s", respStat1.Header.Get("X-Upstream-ID"))
	}


	stat2URL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", statPort2))
	stat2Client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(stat2URL),
		},
		Timeout: 5 * time.Second,
	}
	respStat2, err := stat2Client.Get(targetServer.URL)
	if err != nil {
		t.Fatalf("req through static port 2 failed: %v", err)
	}
	defer respStat2.Body.Close()
	if respStat2.Header.Get("X-Upstream-ID") != "upstream-2" {
		t.Errorf("expected upstream-2, got %s", respStat2.Header.Get("X-Upstream-ID"))
	}
}
