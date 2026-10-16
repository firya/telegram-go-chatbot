package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// The feed URL comes from config.json ("releases_url", default
// DefaultReleasesUrl), so the command can be pointed at a test server.
func TestReleases(t *testing.T) {
	bot, client := setupTestEnv(t)

	yesterday := time.Now().AddDate(0, 0, -1).Format("20060102")
	lastMonth := time.Now().AddDate(0, 0, -30).Format("20060102")
	feed := "BEGIN:VCALENDAR\n" +
		"BEGIN:VEVENT\nDTSTART:" + yesterday + "\nSUMMARY:Новая игра (PS5)\nEND:VEVENT\n" +
		"BEGIN:VEVENT\nDTSTART:" + lastMonth + "\nSUMMARY:Старая игра\nEND:VEVENT\n" +
		"END:VCALENDAR\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, feed)
	}))
	t.Cleanup(server.Close)
	Config.ReleasesUrl = server.URL

	if err := Releases(bot, testContext(bot, "/releases", testUser(42))); err != nil {
		t.Fatalf("Releases: %v", err)
	}

	texts := client.sentTexts()
	want := fmt.Sprintf("%v.%v.%v: Новая игра", yesterday[6:8], yesterday[4:6], yesterday[0:4])
	if len(texts) != 1 || !strings.Contains(texts[0], want) || strings.Contains(texts[0], "Старая игра") {
		t.Errorf("Releases sent %q, want %q and no event older than two days", texts, want)
	}
}

// A dead feed must produce a readable error, not an iCal parse failure.
func TestReleasesFeedError(t *testing.T) {
	bot, _ := setupTestEnv(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "cold start", http.StatusBadGateway)
	}))
	t.Cleanup(server.Close)
	Config.ReleasesUrl = server.URL

	err := Releases(bot, testContext(bot, "/releases", testUser(42)))
	if err == nil || !strings.Contains(err.Error(), "502") {
		t.Errorf("Releases err = %v, want the feed status", err)
	}
}

// "proxy" in config.json is applied through httpClient(); the bare
// http.Get/http.Client calls used to ignore HTTPClientProxy entirely.
func TestHTTPClientUsesProxy(t *testing.T) {
	oldProxy := HTTPClientProxy
	t.Cleanup(func() { HTTPClientProxy = oldProxy })

	var mu sync.Mutex
	var seen string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = r.RequestURI
		mu.Unlock()
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(proxy.Close)

	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatalf("parse proxy url: %v", err)
	}
	HTTPClientProxy = func(*http.Request) (*url.URL, error) { return proxyURL, nil }

	resp, err := httpClient().Get("http://example.invalid/feed.ics")
	if err != nil {
		t.Fatalf("request through proxy: %v", err)
	}
	defer resp.Body.Close()

	mu.Lock()
	defer mu.Unlock()
	if seen != "http://example.invalid/feed.ics" {
		t.Errorf("proxy saw %q, want the absolute target URL", seen)
	}
}
