package proxy

import (
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/kiranimeid/go-rotating-proxy/internal/config"
)

type SelectDialerFunc func() (Dialer, error)

type Handler struct {
	whitelist *config.IPWhitelist
	selector  SelectDialerFunc
}

func NewHandler(whitelist *config.IPWhitelist, selector SelectDialerFunc) *Handler {
	return &Handler{
		whitelist: whitelist,
		selector:  selector,
	}
}

var hopHeaders = []string{
	"Connection",
	"Proxy-Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te",
	"Trailers",
	"Transfer-Encoding",
	"Upgrade",
}

var anonymityHeaders = []string{
	"X-Forwarded-For",
	"Via",
	"Cache-Control",
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.whitelist != nil && !h.whitelist.Contains(r.RemoteAddr) {
		log.Printf("[ACL DENY] client=%s method=%s url=%s", r.RemoteAddr, r.Method, r.URL.String())
		http.Error(w, "Forbidden: IP not allowed", http.StatusForbidden)
		return
	}

	dialer, err := h.selector()
	if err != nil {
		log.Printf("[PROXY ERR] client=%s method=%s url=%s: %v", r.RemoteAddr, r.Method, r.URL.String(), err)
		http.Error(w, "Service Unavailable: "+err.Error(), http.StatusServiceUnavailable)
		return
	}

	if r.Method == http.MethodConnect {
		h.handleConnect(w, r, dialer)
	} else {
		h.handleHTTP(w, r, dialer)
	}
}

func (h *Handler) handleConnect(w http.ResponseWriter, r *http.Request, dialer Dialer) {
	target := r.Host
	if !strings.Contains(target, ":") {
		target = net.JoinHostPort(target, "443")
	}

	log.Printf("[CONNECT] target=%s via=%s (client %s)", target, dialer.String(), r.RemoteAddr)

	upstreamConn, err := dialer.DialContext(r.Context(), "tcp", target)
	if err != nil {
		log.Printf("[CONNECT FAIL] target=%s via=%s: %v", target, dialer.String(), err)
		http.Error(w, "Bad Gateway: "+err.Error(), http.StatusBadGateway)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		upstreamConn.Close()
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		upstreamConn.Close()
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}


	if _, err := clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		clientConn.Close()
		upstreamConn.Close()
		return
	}


	go func() {
		defer clientConn.Close()
		defer upstreamConn.Close()
		_, _ = io.Copy(upstreamConn, clientConn)
	}()

	go func() {
		defer clientConn.Close()
		defer upstreamConn.Close()
		_, _ = io.Copy(clientConn, upstreamConn)
	}()
}

func (h *Handler) handleHTTP(w http.ResponseWriter, r *http.Request, dialer Dialer) {
	outReq := r.Clone(r.Context())
	if r.ContentLength == 0 {
		outReq.Body = nil
	}

	if outReq.URL.Scheme == "" {
		outReq.URL.Scheme = "http"
	}
	if outReq.URL.Host == "" {
		outReq.URL.Host = r.Host
	}
	outReq.RequestURI = ""

	targetURL := outReq.URL.String()
	log.Printf("[HTTP] method=%s url=%s via=%s (client %s)", r.Method, targetURL, dialer.String(), r.RemoteAddr)


	stripHeaders(outReq.Header)

	transport := dialer.Transport()
	if transport == nil {
		transport = http.DefaultTransport.(*http.Transport)
	}


	outReq.Close = true

	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(outReq)
	if err != nil {
		log.Printf("[HTTP FAIL] method=%s url=%s via=%s: %v", r.Method, targetURL, dialer.String(), err)
		http.Error(w, "Bad Gateway: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	log.Printf("[HTTP %d] method=%s url=%s via=%s", resp.StatusCode, r.Method, targetURL, dialer.String())


	stripHeaders(resp.Header)

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func stripHeaders(h http.Header) {
	for _, header := range hopHeaders {
		h.Del(header)
	}
	for _, header := range anonymityHeaders {
		h.Del(header)
	}
}
