# Docker Rotating Proxy (Go)

High-performance rotating proxy server written in Go. Fully optimized for web scraping, automation, and testing workflows (Playwright, Puppeteer, Selenium, curl).

Replaces PHP, Squid, and Gost with a single standalone Go binary or lightweight Docker container.

```
                      Single Go Process
                      -------------------------------------------------
Client (curl)  -----> :3128 (Round-Robin Rotator) ---> Upstream Proxies
                                                            (HTTP / HTTPS / SOCKS5)
Client (Scraper) ---> :30000+ (Dedicated Static Port) -/
```

- **Port 3128**: Rotating proxy endpoint. Every request rotates to the next upstream proxy in round-robin order.
- **Ports 30000+**: Static dedicated proxy endpoints. Each port connects sticky to a specific upstream proxy (useful when a browser session needs the same IP).
- **Supports HTTP & SOCKS5 upstreams**: Both public open proxies and authenticated private proxies (`user:pass`).
- **Scraping Anonymity**: Automatically strips `X-Forwarded-For`, `Via`, and `Cache-Control` headers.
- **Access Control**: Whitelist client IPs or CIDR ranges using `allowed_ip.txt`.

---

## Quick Start

### 1. Configure Upstream Proxies (`proxyList.txt`)

Add your upstream proxies to `proxyList.txt`:
```
IPAddress:Port:Type(socks5 or http or httpsquid):Username:Password
IPAddress:Port:Type(socks5 or http or httpsquid)
IPAddress:Port
```

Example:
```
127.0.0.1:1080:socks5:myUser:myPass
127.0.0.1:44129:httpsquid:squidUser:squidPass
89.187.161.86:8080:http
172.31.22.222:8080
```

### 2. Configure Allowed Client IPs (`allowed_ip.txt`)

Specify client IPs or CIDRs permitted to connect to the proxy in `allowed_ip.txt`. Private networks (`127.0.0.0/8`, `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`) are allowed by default.

```
93.184.216.34
203.0.113.0/24
```

### 3. Run

#### With Docker Compose (Includes Byparr Antibot Solver)
```bash
docker compose up -d
# Rotating Proxy: http://127.0.0.1:3128
# Byparr API:     http://127.0.0.1:8191
```

#### Native Go Binary
```bash
go run ./cmd/proxy
# or build binary
go build -o rotating-proxy ./cmd/proxy
./rotating-proxy -listen=:3128 -proxies=proxyList.txt -allowed=allowed_ip.txt
```

---

## Verification & Usage

### Rotating Requests (Port 3128)
```bash
curl https://httpbin.org/ip --proxy http://127.0.0.1:3128
curl https://httpbin.org/ip --proxy http://127.0.0.1:3128
```

### Sticky / Dedicated Proxies (Port 30000+)
```bash
curl https://httpbin.org/ip --proxy http://127.0.0.1:30000
curl https://httpbin.org/ip --proxy http://127.0.0.1:30001
```

---

## Auto-Update via Cron

To periodically update the proxy list from a public list and reload the service:
```bash
0 * * * * /path/to/public_proxy_cron.sh
```

---

## CLI Options

| Flag | Env Var | Default | Description |
|---|---|---|---|
| `-listen` | `LISTEN_ADDR` | `:3128` | Rotating proxy listen address |
| `-static-port-start` | `STATIC_PORT_START` | `30000` | Starting port for static proxies |
| `-proxies` | `PROXY_LIST` | `proxyList.txt` | Path to proxy list file |
| `-allowed` | `ALLOWED_IP_FILE` | `allowed_ip.txt` | Path to allowed IPs whitelist file |
