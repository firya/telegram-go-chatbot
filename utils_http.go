package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// HTTPTimeout is how long the commands wait for an external service
// (advice, anekdot, the releases feed, file downloads).
const HTTPTimeout = 10 * time.Second

// httpClient builds the client used for outgoing HTTP requests.
// It applies the proxy from ZAVTRA_PROXY: the plain http.Get / &http.Client{}
// calls used to ignore HTTPClientProxy, so the proxy setting did nothing.
func httpClient() *http.Client {
	return &http.Client{
		Timeout: HTTPTimeout,
		Transport: &http.Transport{
			Proxy:               HTTPClientProxy,
			TLSHandshakeTimeout: HTTPTimeout,
		},
	}
}

func DownloadFile(filepath string, url string) (err error) {
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	resp, err := httpClient().Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	return nil
}
