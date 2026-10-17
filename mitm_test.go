package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------- Test-wide certificate authority ----------

// Parts of the bot talk to https:// services: the Bot API file server (media
// conversions, /ai with a photo), anekdot.ru, CoinMarketCap. The helpers below
// intercept those requests, which needs a certificate authority the test
// process trusts, so TestMain generates a throwaway CA, writes it to a file
// and points SSL_CERT_FILE at it - Go adds it to the system trust store. The
// proxy then signs a leaf certificate for every host it is pointed at, and the
// code under test completes an ordinary TLS handshake with a local handler.
//
// The CA is thrown away with the temp dir when the test binary exits, and the
// tests never reach a real service: an unstubbed https:// target answers 404.
var (
	testCA    *x509.Certificate
	testCAKey *ecdsa.PrivateKey

	leafMu    sync.Mutex
	leafCache = map[string]tls.Certificate{}
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "chatbot-test-ca")
	if err != nil {
		fmt.Fprintln(os.Stderr, "test CA: "+err.Error())
		os.Exit(1)
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		fmt.Fprintln(os.Stderr, "test CA key: "+err.Error())
		os.Exit(1)
	}
	now := time.Now()
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "zavtrachat_bot test CA"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &key.PublicKey, key)
	if err != nil {
		fmt.Fprintln(os.Stderr, "test CA cert: "+err.Error())
		os.Exit(1)
	}
	ca, err := x509.ParseCertificate(der)
	if err != nil {
		fmt.Fprintln(os.Stderr, "test CA parse: "+err.Error())
		os.Exit(1)
	}
	caFile := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "test CA write: "+err.Error())
		os.Exit(1)
	}

	testCA, testCAKey = ca, key
	// Go reads SSL_CERT_FILE while building the system certificate pool, so it
	// has to be set before the first TLS request of the test binary.
	os.Setenv("SSL_CERT_FILE", caFile)

	// The proxy below is started before any test runs, because Go caches the
	// proxy settings of the default transport on its first use: a proxy set up
	// per test would be ignored by clients that build their own http.Client.
	envProxy := httptest.NewServer(&mitmProxy{handler: serveEnvStub})
	os.Setenv("HTTP_PROXY", envProxy.URL)
	os.Setenv("HTTPS_PROXY", envProxy.URL)
	// yt-dlp follows the proxy variables as well, and the media fixtures of the
	// yt-dlp tests are served on the loopback address: loopback stays direct.
	os.Setenv("NO_PROXY", "127.0.0.1,localhost")
	os.Setenv("no_proxy", "127.0.0.1,localhost")

	code := m.Run()

	envProxy.Close()
	os.RemoveAll(dir)
	os.Exit(code)
}

// mitmLeaf signs (and caches) a certificate for one host with the test CA.
func mitmLeaf(host string) (tls.Certificate, error) {
	leafMu.Lock()
	defer leafMu.Unlock()

	if cert, ok := leafCache[host]; ok {
		return cert, nil
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	now := time.Now()
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(now.UnixNano()),
		Subject:      pkix.Name{CommonName: host},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if ip := net.ParseIP(host); ip != nil {
		tmpl.IPAddresses = []net.IP{ip}
	} else {
		tmpl.DNSNames = []string{host}
	}

	der, err := x509.CreateCertificate(rand.Reader, &tmpl, testCA, &key.PublicKey, testCAKey)
	if err != nil {
		return tls.Certificate{}, err
	}
	cert := tls.Certificate{Certificate: [][]byte{der, testCA.Raw}, PrivateKey: key}
	leafCache[host] = cert
	return cert, nil
}

// ---------- CONNECT-capable proxy ----------

// mitmProxy answers plain requests with the test handler, and terminates
// CONNECT tunnels as a TLS server with a leaf certificate for the requested
// host, handing the decrypted requests to the same handler.
type mitmProxy struct {
	handler http.HandlerFunc

	mu      sync.Mutex
	servers []*http.Server
}

func (p *mitmProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodConnect {
		p.handler(w, r)
		return
	}

	host, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		host = r.Host
	}

	conn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		return
	}
	if _, err := io.WriteString(conn, "HTTP/1.1 200 Connection established\r\n\r\n"); err != nil {
		conn.Close()
		return
	}

	cert, err := mitmLeaf(host)
	if err != nil {
		conn.Close()
		return
	}
	cfg := &tls.Config{Certificates: []tls.Certificate{cert}}

	server := &http.Server{Handler: p.handler}
	p.mu.Lock()
	p.servers = append(p.servers, server)
	p.mu.Unlock()

	// Serve takes the tunnel as its single connection and returns once that
	// connection is closed (see singleConnListener).
	_ = server.Serve(&singleConnListener{conn: tls.Server(conn, cfg), closed: make(chan struct{})})
}

func (p *mitmProxy) closeServers() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, server := range p.servers {
		server.Close()
	}
}

// singleConnListener hands one already-decrypted connection to an http.Server
// and unblocks Accept when that connection closes, so Serve returns instead of
// idling on a tunnel nobody is using any more.
type singleConnListener struct {
	conn   net.Conn
	closed chan struct{}
	taken  atomic.Bool
	once   sync.Once
}

func (l *singleConnListener) Accept() (net.Conn, error) {
	if l.taken.CompareAndSwap(false, true) {
		return &closeNotifyConn{Conn: l.conn, onClosed: l.close}, nil
	}
	<-l.closed
	return nil, io.EOF
}

func (l *singleConnListener) Close() error {
	err := l.conn.Close()
	l.close()
	return err
}

func (l *singleConnListener) Addr() net.Addr { return l.conn.LocalAddr() }

func (l *singleConnListener) close() { l.once.Do(func() { close(l.closed) }) }

// closeNotifyConn reports the close of the tunnel back to the listener.
type closeNotifyConn struct {
	net.Conn
	onClosed func()
	once     sync.Once
}

func (c *closeNotifyConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(c.onClosed)
	return err
}

// stubTLSProxy points httpClient() at a CONNECT-capable test proxy, so code
// that requests an https:// URL is served by the handler instead of leaving
// the process. The handler sees the target host in r.Host.
func stubTLSProxy(t *testing.T, handler http.HandlerFunc) {
	t.Helper()

	proxy := &mitmProxy{handler: handler}
	old := HTTPClientProxy
	server := httptest.NewServer(proxy)

	// The tunnels are closed before the proxy they were served through, and the
	// proxy setting before anything that may still be reading it.
	t.Cleanup(func() { HTTPClientProxy = old })
	t.Cleanup(server.Close)
	t.Cleanup(proxy.closeServers)

	proxyURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("stubTLSProxy: %v", err)
	}
	HTTPClientProxy = func(*http.Request) (*url.URL, error) { return proxyURL, nil }
}

// ---------- Routes of the process-wide proxy ----------

// stubEnvRoute serves one host/path through the proxy TestMain published in
// HTTP_PROXY/HTTPS_PROXY. It is needed by code that builds its own
// http.Client with the default transport (the CoinMarketCap client does) and
// so cannot be redirected through HTTPClientProxy.
type envStubRoute struct {
	id      int
	host    string
	prefix  string
	handler http.HandlerFunc
}

var (
	envStubMu     sync.Mutex
	envStubRoutes []envStubRoute
	envStubID     int
)

func stubEnvRoute(t *testing.T, host, prefix string, handler http.HandlerFunc) {
	t.Helper()

	envStubMu.Lock()
	envStubID++
	route := envStubRoute{id: envStubID, host: host, prefix: prefix, handler: handler}
	envStubRoutes = append(envStubRoutes, route)
	envStubMu.Unlock()

	t.Cleanup(func() {
		envStubMu.Lock()
		defer envStubMu.Unlock()
		for i, r := range envStubRoutes {
			if r.id == route.id {
				envStubRoutes = append(envStubRoutes[:i], envStubRoutes[i+1:]...)
				return
			}
		}
	})
}

func serveEnvStub(w http.ResponseWriter, r *http.Request) {
	envStubMu.Lock()
	// The most specific registered prefix wins, so a test can override one
	// endpoint of a host another test already stubbed.
	var handler http.HandlerFunc
	var bestPrefix string
	for _, route := range envStubRoutes {
		if r.Host == route.host && strings.HasPrefix(r.URL.Path, route.prefix) &&
			len(route.prefix) > len(bestPrefix) {
			bestPrefix = route.prefix
			handler = route.handler
		}
	}
	envStubMu.Unlock()

	if handler == nil {
		http.Error(w, "no test stub for "+r.Host+r.URL.Path, http.StatusNotFound)
		return
	}
	handler(w, r)
}
