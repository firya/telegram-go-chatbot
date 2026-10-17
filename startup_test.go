package main

import (
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"gorm.io/gorm"
)

// TestStartupReport covers the startup notice: the operational facts have to
// be in it, and nothing that authenticates anything — the token is not in it
// at all, the DSN and the proxy keep their host and user name but lose the
// password, the app hash and the API keys are reported as presence.
func TestStartupReport(t *testing.T) {
	bot, _ := setupTestEnv(t)

	Config.Token = "12345:SECRETTOKEN"
	Config.DSN = "postgres://zavtra:SECRETPASS@db.example:5432/zavtra"
	Config.AppID = 777
	Config.AppHash = "SECRETHASH"
	Config.CurrencyKey = "SECRETKEY"
	Config.AIURL = "http://ai.example/v1"
	Config.AIModel = "gpt-test"
	Config.AIToken = "SECRETAITOKEN"
	Config.Proxy = "http://user:SECRETPASS@proxy.example:3128"
	Config.BotApiUrl = "http://127.0.0.1:8081"
	Config.BotApiDataDir = "/var/lib/telegram-bot-api"
	Config.FontPath = "/usr/share/fonts/test.ttf"
	Config.Listen = "127.0.0.1:8080"
	Config.EndpointPublicURL = "http://example.invalid"
	Config.MaxConnections = 40

	text := startupReport(bot, "webhook")

	for _, want := range []string{
		"has finished starting up",
		"@testbot (1)",
		"Connection: webhook: listen 127.0.0.1:8080, url http://example.invalid, max 40",
		"sqlite, postgres://zavtra@db.example:5432/zavtra (",
		"Bot API: http://127.0.0.1:8081, datadir /var/lib/telegram-bot-api",
		"Chats: main -100, reserve -200, comments -300, channel -, stream -",
		"Moderation: admins 1, moders 1, sysadmin 100, antispam 3 msg per 30s",
		"Updates: -",
		"config .env",
		"proxy http://user@proxy.example:3128",
		"font /usr/share/fonts/test.ttf",
		"app id 777, app hash set, currency key set",
		"AI http://ai.example/v1, model gpt-test, AI token set",
		"rev ",
		runtime.Version(),
	} {
		if !strings.Contains(text, want) {
			t.Errorf("startup report has no %q:\n%v", want, text)
		}
	}

	for _, secret := range []string{
		"SECRETTOKEN",
		"SECRETPASS",
		"SECRETHASH",
		"SECRETKEY",
		"SECRETAITOKEN",
		Config.Token,
	} {
		if strings.Contains(text, secret) {
			t.Errorf("startup report leaked %q:\n%v", secret, text)
		}
	}

	// Polling needs no address, and a bot without a database says so.
	if polling := startupReport(bot, "polling"); !strings.Contains(polling, "Connection: polling") {
		t.Errorf("polling report = %q, want the plain polling line", polling)
	}

	DB = gorm.DB{}
	Config.DSN = ""
	if empty := startupReport(bot, "polling"); !strings.Contains(empty, "Database: not initialized") {
		t.Errorf("report without a database = %q, want %q", empty, "Database: not initialized")
	}

	// ZAVTRA_ENV_FILE names the config file the report prints.
	t.Setenv("ZAVTRA_ENV_FILE", "/etc/zavtra.env")
	if named := startupReport(bot, "polling"); !strings.Contains(named, "config /etc/zavtra.env") {
		t.Errorf("report with ZAVTRA_ENV_FILE = %q, want it to name /etc/zavtra.env", named)
	}
}

// TestBuildInfo covers the build line of the report: the toolchain and the
// platform are always there, the commit only when Go stamped it into the
// binary (a test binary is not stamped, so the revision reads "unknown").
func TestBuildInfo(t *testing.T) {
	info := buildInfo()

	for _, want := range []string{"rev ", runtime.Version(), runtime.GOOS, runtime.GOARCH} {
		if !strings.Contains(info, want) {
			t.Errorf("buildInfo = %q, want it to contain %q", info, want)
		}
	}
}

// TestRedactDSN covers the DSN forms the report has to sanitize: a URL DSN
// keeps the user name and drops the password, a keyword DSN keeps everything
// but the password, a local path has nothing to drop.
func TestRedactDSN(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"postgres://zavtra:secret@db.example:5432/zavtra?sslmode=disable", "postgres://zavtra@db.example:5432/zavtra?sslmode=disable"},
		{"host=localhost user=zavtra password=secret dbname=zavtra port=5432", "host=localhost user=zavtra password=*** dbname=zavtra port=5432"},
		{"file:/var/lib/zavtra/zavtra.db?mode=rwc", "file:/var/lib/zavtra/zavtra.db?mode=rwc"},
		{"", "-"},
	} {
		if got := redactDSN(c.in); got != c.want {
			t.Errorf("redactDSN(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestRedactProxy covers the proxy forms: with credentials the password goes
// away, a plain host:port is reported as it is.
func TestRedactProxy(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"http://user:secret@proxy.example:3128", "http://user@proxy.example:3128"},
		{"proxy.example:3128", "proxy.example:3128"},
		{"", "-"},
	} {
		if got := redactProxy(c.in); got != c.want {
			t.Errorf("redactProxy(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestFfmpegVersion reports the version line of the installed ffmpeg, and
// "Unknown" when the binary is not on the machine. The version itself is not
// pinned, only its shape.
func TestFfmpegVersion(t *testing.T) {
	_, lookErr := exec.LookPath("ffmpeg")
	got := ffmpegVersion()

	if lookErr != nil {
		if got != "Unknown" {
			t.Errorf("ffmpegVersion = %q, want %q without ffmpeg", got, "Unknown")
		}
		t.Skip("ffmpeg is not installed, the version line is not checked")
	}

	if got == "Unknown" || strings.HasPrefix(got, "ffmpeg version") {
		t.Errorf("ffmpegVersion = %q, want the bare version of the installed ffmpeg", got)
	}
}
