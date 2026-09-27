package proxy

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kiranimeid/go-rotating-proxy/internal/config"
)

func TestHandlerIPWhitelist(t *testing.T) {
	wl := config.NewIPWhitelist()
	handler := NewHandler(wl, func() (Dialer, error) {
		return NewDirectDialer(), nil
	})


	reqAllowed := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	reqAllowed.RemoteAddr = "127.0.0.1:12345"
	recAllowed := httptest.NewRecorder()
	handler.ServeHTTP(recAllowed, reqAllowed)
	if recAllowed.Code == http.StatusForbidden {
		t.Fatalf("expected allowed localhost, got 403")
	}


	reqDenied := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	reqDenied.RemoteAddr = "8.8.8.8:12345"
	recDenied := httptest.NewRecorder()
	handler.ServeHTTP(recDenied, reqDenied)
	if recDenied.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for 8.8.8.8, got %d", recDenied.Code)
	}
}

func TestHandlerHTTPForwardingAndAnonymityHeaders(t *testing.T) {

	var receivedHeaders http.Header
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.Header().Set("X-Forwarded-For", "strip-me-upstream")
		w.Header().Set("Via", "strip-me-via")
		w.Header().Set("X-Custom-Echo", "kept")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok-body"))
	}))
	defer targetServer.Close()

	handler := NewHandler(nil, func() (Dialer, error) {
		return NewDirectDialer(), nil
	})

	req := httptest.NewRequest(http.MethodGet, targetServer.URL+"/testpath", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.Header.Set("Via", "1.1 squid")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("User-Agent", "TestScraper/1.0")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}


	if receivedHeaders.Get("X-Forwarded-For") != "" {
		t.Errorf("expected X-Forwarded-For to be stripped from target request")
	}
	if receivedHeaders.Get("Via") != "" {
		t.Errorf("expected Via to be stripped from target request")
	}
	if receivedHeaders.Get("Cache-Control") != "" {
		t.Errorf("expected Cache-Control to be stripped from target request")
	}
	if receivedHeaders.Get("User-Agent") != "TestScraper/1.0" {
		t.Errorf("expected User-Agent preserved, got %s", receivedHeaders.Get("User-Agent"))
	}


	if rec.Header().Get("X-Forwarded-For") != "" {
		t.Errorf("expected X-Forwarded-For to be stripped from response")
	}
	if rec.Header().Get("Via") != "" {
		t.Errorf("expected Via to be stripped from response")
	}
	if rec.Header().Get("X-Custom-Echo") != "kept" {
		t.Errorf("expected X-Custom-Echo preserved, got %s", rec.Header().Get("X-Custom-Echo"))
	}
	if rec.Body.String() != "ok-body" {
		t.Errorf("expected body 'ok-body', got %s", rec.Body.String())
	}
}

func TestHandlerConnectTunnel(t *testing.T) {

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer l.Close()

	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 4)
		_, _ = io.ReadFull(conn, buf)
		_, _ = conn.Write([]byte("PONG"))
	}()

	proxyServer := httptest.NewServer(NewHandler(nil, func() (Dialer, error) {
		return NewDirectDialer(), nil
	}))
	defer proxyServer.Close()


	conn, err := net.Dial("tcp", proxyServer.Listener.Addr().String())
	if err != nil {
		t.Fatalf("dial proxy failed: %v", err)
	}
	defer conn.Close()

	req := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", l.Addr().String(), l.Addr().String())
	_, _ = conn.Write([]byte(req))

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	if err != nil {
		t.Fatalf("read CONNECT response failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}


	_, err = conn.Write([]byte("PING"))
	if err != nil {
		t.Fatalf("write through tunnel failed: %v", err)
	}

	res := make([]byte, 4)
	_, err = io.ReadFull(conn, res)
	if err != nil {
		t.Fatalf("read through tunnel failed: %v", err)
	}
	if string(res) != "PONG" {
		t.Fatalf("expected PONG, got %s", string(res))
	}
}
