package main

import (
	"net/http"
	"time"
)

// HTTPTimeout is how long the commands wait for an external service
// (advice, anekdot, the releases feed, file downloads).
const HTTPTimeout = 10 * time.Second

// httpClient builds the client used for outgoing HTTP requests.
// It applies the proxy from config.json: the plain http.Get / &http.Client{}
// calls used to ignore HTTPClientProxy, so the "proxy" setting did nothing.
func httpClient() *http.Client {
	return &http.Client{
		Timeout: HTTPTimeout,
		Transport: &http.Transport{
			Proxy:               HTTPClientProxy,
			TLSHandshakeTimeout: HTTPTimeout,
		},
	}
}
