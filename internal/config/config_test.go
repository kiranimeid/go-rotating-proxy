package config

import (
	"strings"
	"testing"
)

func TestParseProxyList(t *testing.T) {
	input := `
# Comment line
127.0.0.1:8080
127.0.0.1:1080:socks5
127.0.0.1:44129:httpsquid:myuser:mypass
89.187.161.86:80:http:user2:pass2
invalid-line
:9999
1.2.3.4:notaport
`
	proxies, err := ParseProxyList(strings.NewReader(input), 30000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(proxies) != 4 {
		t.Fatalf("expected 4 proxies, got %d", len(proxies))
	}

	
	if proxies[0].Host != "127.0.0.1" || proxies[0].Port != 8080 || proxies[0].Scheme != "http" || proxies[0].StaticPort != 30000 {
		t.Errorf("proxy 0 mismatch: %+v", proxies[0])
	}

	
	if proxies[1].Host != "127.0.0.1" || proxies[1].Port != 1080 || proxies[1].Scheme != "socks5" || proxies[1].StaticPort != 30001 {
		t.Errorf("proxy 1 mismatch: %+v", proxies[1])
	}

	
	if proxies[2].Scheme != "httpsquid" || proxies[2].User != "myuser" || proxies[2].Pass != "mypass" || proxies[2].StaticPort != 30002 {
		t.Errorf("proxy 2 mismatch: %+v", proxies[2])
	}

	
	if proxies[3].Scheme != "http" || proxies[3].User != "user2" || proxies[3].Pass != "pass2" || proxies[3].StaticPort != 30003 {
		t.Errorf("proxy 3 mismatch: %+v", proxies[3])
	}
}

func TestIPWhitelist(t *testing.T) {
	input := `
93.184.216.34
203.0.113.0/24
`
	wl, err := ParseAllowedIPs(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	
	if !wl.Contains("127.0.0.1:45321") {
		t.Errorf("expected localhost to be allowed")
	}
	if !wl.Contains("[::1]:1234") {
		t.Errorf("expected IPv6 loopback to be allowed")
	}
	
	if !wl.Contains("192.168.1.100:80") {
		t.Errorf("expected 192.168.1.100 to be allowed")
	}
	
	if !wl.Contains("93.184.216.34:54321") {
		t.Errorf("expected 93.184.216.34 to be allowed")
	}
	
	if !wl.Contains("203.0.113.42:8080") {
		t.Errorf("expected 203.0.113.42 to be allowed")
	}
	
	if wl.Contains("8.8.8.8:53") {
		t.Errorf("expected 8.8.8.8 to be denied")
	}
}
