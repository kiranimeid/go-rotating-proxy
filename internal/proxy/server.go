package proxy

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kiranimeid/go-rotating-proxy/internal/config"
)


type Server struct {
	listenAddr string
	whitelist  *config.IPWhitelist
	proxies    []config.Proxy
	dialers    []Dialer
	counter    atomic.Uint64
	servers    []*http.Server
	mu         sync.Mutex
}


type Config struct {
	ListenAddr string
	Whitelist  *config.IPWhitelist
	Proxies    []config.Proxy
}


func NewServer(cfg Config) (*Server, error) {
	if len(cfg.Proxies) == 0 {
		return nil, errors.New("no upstream proxies configured")
	}

	dialers := make([]Dialer, len(cfg.Proxies))
	for i, p := range cfg.Proxies {
		d, err := NewDialer(p)
		if err != nil {
			return nil, fmt.Errorf("failed to init dialer for proxy %s:%d (%s): %w", p.Host, p.Port, p.Scheme, err)
		}
		dialers[i] = d
	}

	return &Server{
		listenAddr: cfg.ListenAddr,
		whitelist:  cfg.Whitelist,
		proxies:    cfg.Proxies,
		dialers:    dialers,
	}, nil
}


func (s *Server) nextDialer() (Dialer, error) {
	idx := s.counter.Add(1) - 1
	selected := idx % uint64(len(s.dialers))
	return s.dialers[selected], nil
}


func (s *Server) Start(ctx context.Context) error {
	var listeners []net.Listener
	var servers []*http.Server


	rotListener, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on rotating port %s: %w", s.listenAddr, err)
	}
	listeners = append(listeners, rotListener)

	rotHandler := NewHandler(s.whitelist, s.nextDialer)
	rotServer := &http.Server{
		Handler:      rotHandler,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
	}
	servers = append(servers, rotServer)
	log.Printf("Rotating proxy listening on %s (round-robin across %d upstreams)", s.listenAddr, len(s.dialers))


	for i, p := range s.proxies {
		staticAddr := fmt.Sprintf(":%d", p.StaticPort)
		l, err := net.Listen("tcp", staticAddr)
		if err != nil {

			for _, prev := range listeners {
				prev.Close()
			}
			return fmt.Errorf("failed to listen on static port %s: %w", staticAddr, err)
		}
		listeners = append(listeners, l)

		d := s.dialers[i]
		h := NewHandler(s.whitelist, func() (Dialer, error) {
			return d, nil
		})
		srv := &http.Server{
			Handler:      h,
			ReadTimeout:  60 * time.Second,
			WriteTimeout: 60 * time.Second,
		}
		servers = append(servers, srv)
		log.Printf("Static proxy %d bound to %s -> %s://%s:%d", i+1, staticAddr, p.Scheme, p.Host, p.Port)
	}

	s.mu.Lock()
	s.servers = servers
	s.mu.Unlock()

	errChan := make(chan error, len(servers))
	for i := range servers {
		srv := servers[i]
		l := listeners[i]
		go func() {
			if err := srv.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errChan <- err
			}
		}()
	}

	select {
	case <-ctx.Done():
		log.Printf("Shutdown signal received, closing listeners...")
		return s.Shutdown(context.Background())
	case err := <-errChan:
		_ = s.Shutdown(context.Background())
		return err
	}
}


func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var firstErr error
	for _, srv := range s.servers {
		if err := srv.Shutdown(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
