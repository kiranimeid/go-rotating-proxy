package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/kiranimeid/go-rotating-proxy/internal/config"
	"github.com/kiranimeid/go-rotating-proxy/internal/proxy"
)

func main() {
	listenAddr := flag.String("listen", envOrDefault("LISTEN_ADDR", ":3128"), "Rotating proxy listen address")
	staticPortStartStr := envOrDefault("STATIC_PORT_START", "30000")
	staticPortStartDef, _ := strconv.Atoi(staticPortStartStr)
	staticPortStart := flag.Int("static-port-start", staticPortStartDef, "Starting port for static dedicated proxy listeners")
	proxyListPath := flag.String("proxies", envOrDefault("PROXY_LIST", "proxyList.txt"), "Path to proxyList.txt")
	allowedIPPath := flag.String("allowed", envOrDefault("ALLOWED_IP_FILE", "allowed_ip.txt"), "Path to allowed_ip.txt")
	flag.Parse()

	log.Println("Starting Docker Rotating Proxy (Go)")

	whitelist, err := config.LoadAllowedIPsFile(*allowedIPPath)
	if err != nil {
		log.Fatalf("Failed to load allowed IP whitelist from %s: %v", *allowedIPPath, err)
	}

	proxies, err := config.LoadProxyListFile(*proxyListPath, *staticPortStart)
	if err != nil {
		log.Fatalf("Failed to load proxy list from %s: %v", *proxyListPath, err)
	}
	if len(proxies) == 0 {
		log.Fatalf("No valid proxies found in %s", *proxyListPath)
	}
	log.Printf("Loaded %d proxies from %s", len(proxies), *proxyListPath)

	srv, err := proxy.NewServer(proxy.Config{
		ListenAddr: *listenAddr,
		Whitelist:  whitelist,
		Proxies:    proxies,
	})
	if err != nil {
		log.Fatalf("Failed to initialize server: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		log.Fatalf("Server stopped with error: %v", err)
	}
	log.Println("Server stopped gracefully")
}

func envOrDefault(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
