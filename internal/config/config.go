package config

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
)


type Proxy struct {
	Host       string
	Port       int
	Scheme     string
	User       string
	Pass       string
	StaticPort int
}


func (p Proxy) Addr() string {
	return net.JoinHostPort(p.Host, strconv.Itoa(p.Port))
}


func ParseProxyList(r io.Reader, startStaticPort int) ([]Proxy, error) {
	var proxies []Proxy
	scanner := bufio.NewScanner(r)
	portOffset := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Split(line, ":")
		if len(parts) < 2 {
			continue
		}

		host := parts[0]
		port, err := strconv.Atoi(parts[1])
		if err != nil || host == "" || port <= 0 || port > 65535 {
			continue
		}

		scheme := "http"
		var user, pass string

		if len(parts) >= 3 && parts[2] != "" {
			scheme = strings.ToLower(parts[2])
		}
		if len(parts) >= 5 {
			user = parts[3]
			pass = parts[4]
		}

		proxies = append(proxies, Proxy{
			Host:       host,
			Port:       port,
			Scheme:     scheme,
			User:       user,
			Pass:       pass,
			StaticPort: startStaticPort + portOffset,
		})
		portOffset++
	}

	return proxies, scanner.Err()
}


func LoadProxyListFile(path string, startStaticPort int) ([]Proxy, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseProxyList(f, startStaticPort)
}


type IPWhitelist struct {
	subnets []*net.IPNet
	ips     []net.IP
}

var defaultPrivateSubnets = []string{
	"127.0.0.0/8",
	"::1/128",
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"fc00::/7",
	"fe80::/10",
}


func NewIPWhitelist() *IPWhitelist {
	w := &IPWhitelist{}
	for _, cidr := range defaultPrivateSubnets {
		_, subnet, err := net.ParseCIDR(cidr)
		if err == nil {
			w.subnets = append(w.subnets, subnet)
		}
	}
	return w
}


func ParseAllowedIPs(r io.Reader) (*IPWhitelist, error) {
	w := NewIPWhitelist()
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.Contains(line, "/") {
			_, subnet, err := net.ParseCIDR(line)
			if err != nil {
				return nil, fmt.Errorf("invalid CIDR %q: %w", line, err)
			}
			w.subnets = append(w.subnets, subnet)
		} else {
			ip := net.ParseIP(line)
			if ip == nil {
				return nil, fmt.Errorf("invalid IP %q", line)
			}
			w.ips = append(w.ips, ip)
		}
	}

	return w, scanner.Err()
}


func LoadAllowedIPsFile(path string) (*IPWhitelist, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			
			return NewIPWhitelist(), nil
		}
		return nil, err
	}
	defer f.Close()
	return ParseAllowedIPs(f)
}


func (w *IPWhitelist) Contains(remoteAddr string) bool {
	host := remoteAddr
	if h, _, err := net.SplitHostPort(remoteAddr); err == nil {
		host = h
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	for _, direct := range w.ips {
		if direct.Equal(ip) {
			return true
		}
	}

	for _, subnet := range w.subnets {
		if subnet.Contains(ip) {
			return true
		}
	}

	return false
}
