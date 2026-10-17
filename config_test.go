package main

import (
	"os"
	"strings"
	"testing"
)

// clearZavtraEnv removes every ZAVTRA_* variable from the environment for the
// duration of the test, so a .env of the developer running the suite cannot
// leak into a case that expects an empty environment.
func clearZavtraEnv(t *testing.T) {
	t.Helper()

	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "ZAVTRA_") {
			value := os.Getenv(key)
			os.Unsetenv(key)
			t.Cleanup(func() { os.Setenv(key, value) })
		}
	}
}

// writeTestEnv drops a .env file into the test working directory.
func writeTestEnv(t *testing.T, contents string) {
	t.Helper()

	if err := os.WriteFile(".env", []byte(contents), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
}

// TestConfigInit covers the environment based configuration: the defaults, the
// mandatory token and chat, the .env file, the environment winning over it,
// lists, numbers and the proxy. Every case runs in its own directory, so the
// .env of the checkout is never touched.
func TestConfigInit(t *testing.T) {
	oldConfig, oldProxy := Config, HTTPClientProxy
	t.Cleanup(func() { Config, HTTPClientProxy = oldConfig, oldProxy })

	t.Run("defaults", func(t *testing.T) {
		t.Chdir(t.TempDir())
		clearZavtraEnv(t)
		Config, HTTPClientProxy = &Configuration{}, nil

		err := ConfigInit()
		if err == nil || !strings.Contains(err.Error(), "ZAVTRA_TOKEN is not set") {
			t.Errorf("ConfigInit: err = %v, want the missing token error", err)
		}

		// A missing .env is not an error, and nothing is written: the
		// environment alone is a complete configuration.
		if _, err := os.Stat(".env"); !os.IsNotExist(err) {
			t.Errorf("ConfigInit touched .env: %v", err)
		}

		// The mandatory fields stop the run, so the defaults are checked with
		// a token and a chat.
		t.Setenv("ZAVTRA_TOKEN", "123:abc")
		t.Setenv("ZAVTRA_CHAT", "-100")
		if err := ConfigInit(); err != nil {
			t.Fatalf("ConfigInit: %v", err)
		}
		if Config.BotApiUrl != DefaultBotApiUrl {
			t.Errorf("BotApiUrl = %q, want the default Bot API URL", Config.BotApiUrl)
		}
		if len(Config.AllowedUpdates) != 4 || Config.AllowedUpdates[0] != "message" {
			t.Errorf("AllowedUpdates = %v, want the four default update types", Config.AllowedUpdates)
		}
		if len(Config.Admins) != 0 || len(Config.Moders) != 0 {
			t.Errorf("Admins/Moders = %v/%v, want empty lists", Config.Admins, Config.Moders)
		}
		if Config.ReleasesUrl != DefaultReleasesUrl {
			t.Errorf("ReleasesUrl = %q, want the default release calendar", Config.ReleasesUrl)
		}
		if Config.AntiSpamWindow != 0 || Config.SysAdmin != 0 {
			t.Errorf("antispam window = %v, sysadmin = %v, want 0 for unset variables",
				Config.AntiSpamWindow, Config.SysAdmin)
		}
		if HTTPClientProxy != nil {
			t.Errorf("HTTPClientProxy is set, want no proxy transport without a proxy")
		}
	})

	t.Run("token and chat are mandatory", func(t *testing.T) {
		t.Chdir(t.TempDir())
		clearZavtraEnv(t)
		Config, HTTPClientProxy = &Configuration{}, nil

		t.Setenv("ZAVTRA_TOKEN", "123:abc")
		err := ConfigInit()
		if err == nil || !strings.Contains(err.Error(), "ZAVTRA_CHAT is not set") {
			t.Errorf("ConfigInit: err = %v, want the missing chat error", err)
		}

		t.Setenv("ZAVTRA_CHAT", "-100")
		t.Setenv("ZAVTRA_TOKEN", "")
		err = ConfigInit()
		if err == nil || !strings.Contains(err.Error(), "ZAVTRA_TOKEN is not set") {
			t.Errorf("ConfigInit: err = %v, want the missing token error", err)
		}
	})

	t.Run("env file", func(t *testing.T) {
		t.Chdir(t.TempDir())
		clearZavtraEnv(t)
		Config, HTTPClientProxy = &Configuration{}, nil
		writeTestEnv(t, `# the bot token
ZAVTRA_TOKEN=123:abc

ZAVTRA_CHAT=-100
ZAVTRA_RESERVE_CHAT=-200
ZAVTRA_SYSADMIN=7
ZAVTRA_ADMINS=1,2
ZAVTRA_MODERS=3
ZAVTRA_ANTISPAM_WINDOW=15
ZAVTRA_ANTISPAM_COUNT=4
ZAVTRA_RELEASES_URL=https://example.invalid/cal
export ZAVTRA_CURRENCY_KEY=cmc-key
`)

		if err := ConfigInit(); err != nil {
			t.Fatalf("ConfigInit: %v", err)
		}
		if Config.Token != "123:abc" {
			t.Errorf("Token = %q, want the token from the file", Config.Token)
		}
		if Config.Chat != -100 || Config.ReserveChat != -200 || Config.SysAdmin != 7 {
			t.Errorf("chats = %v/%v, sysadmin = %v, want the values from the file",
				Config.Chat, Config.ReserveChat, Config.SysAdmin)
		}
		if len(Config.Admins) != 2 || len(Config.Moders) != 1 || Config.Moders[0] != 3 {
			t.Errorf("Admins/Moders = %v/%v, want the lists from the file", Config.Admins, Config.Moders)
		}
		if Config.AntiSpamWindow != 15 || Config.AntiSpamCount != 4 {
			t.Errorf("antispam = %v/%v, want 15/4", Config.AntiSpamWindow, Config.AntiSpamCount)
		}
		if Config.ReleasesUrl != "https://example.invalid/cal" {
			t.Errorf("ReleasesUrl = %q, want the configured calendar", Config.ReleasesUrl)
		}
		if Config.CurrencyKey != "cmc-key" {
			t.Errorf("CurrencyKey = %q, want the key of the export line", Config.CurrencyKey)
		}
	})

	t.Run("environment wins over env file", func(t *testing.T) {
		t.Chdir(t.TempDir())
		clearZavtraEnv(t)
		Config, HTTPClientProxy = &Configuration{}, nil
		writeTestEnv(t, "ZAVTRA_TOKEN=file:token\nZAVTRA_CHAT=-100\nZAVTRA_AI_MODEL=file-model\n")
		t.Setenv("ZAVTRA_TOKEN", "env:token")

		if err := ConfigInit(); err != nil {
			t.Fatalf("ConfigInit: %v", err)
		}
		if Config.Token != "env:token" {
			t.Errorf("Token = %q, want the token of the environment, not of the file", Config.Token)
		}
		if Config.AIModel != "file-model" {
			t.Errorf("AIModel = %q, want the value the environment does not set", Config.AIModel)
		}
	})

	t.Run("env file path", func(t *testing.T) {
		t.Chdir(t.TempDir())
		clearZavtraEnv(t)
		Config, HTTPClientProxy = &Configuration{}, nil
		if err := os.WriteFile("bot.env", []byte("ZAVTRA_TOKEN=999:xyz\nZAVTRA_CHAT=-300\n"), 0o600); err != nil {
			t.Fatalf("write bot.env: %v", err)
		}
		t.Setenv("ZAVTRA_ENV_FILE", "bot.env")

		if err := ConfigInit(); err != nil {
			t.Fatalf("ConfigInit: %v", err)
		}
		if Config.Token != "999:xyz" || Config.Chat != -300 {
			t.Errorf("token/chat = %q/%v, want the values of ZAVTRA_ENV_FILE", Config.Token, Config.Chat)
		}
	})

	t.Run("lists", func(t *testing.T) {
		t.Chdir(t.TempDir())
		clearZavtraEnv(t)
		Config, HTTPClientProxy = &Configuration{}, nil
		t.Setenv("ZAVTRA_TOKEN", "123:abc")
		t.Setenv("ZAVTRA_CHAT", "-100")
		t.Setenv("ZAVTRA_ADMINS", "1, 2;3\t4")
		t.Setenv("ZAVTRA_ALLOWED_UPDATES", "message,chat_member")
		t.Setenv("ZAVTRA_AI_ADMIN_ONLY_TOOLS", "search,calc")

		if err := ConfigInit(); err != nil {
			t.Fatalf("ConfigInit: %v", err)
		}
		if len(Config.Admins) != 4 || Config.Admins[3] != 4 {
			t.Errorf("Admins = %v, want the four ids of the list", Config.Admins)
		}
		if len(Config.AllowedUpdates) != 2 || Config.AllowedUpdates[1] != "chat_member" {
			t.Errorf("AllowedUpdates = %v, want the two update types of the list", Config.AllowedUpdates)
		}
		if len(Config.AIAdminOnlyTools) != 2 {
			t.Errorf("AIAdminOnlyTools = %v, want the two tools of the list", Config.AIAdminOnlyTools)
		}
	})

	t.Run("quoted values", func(t *testing.T) {
		t.Chdir(t.TempDir())
		clearZavtraEnv(t)
		Config, HTTPClientProxy = &Configuration{}, nil
		writeTestEnv(t, "ZAVTRA_TOKEN=\"123:abc$dollar\"\nZAVTRA_CHAT=-100\n"+
			"ZAVTRA_DSN=\"postgres://u:p#1@localhost/db\"\n"+
			"ZAVTRA_AI_SYSTEM=\"first line\nsecond line\"\n")

		if err := ConfigInit(); err != nil {
			t.Fatalf("ConfigInit: %v", err)
		}
		if Config.Token != "123:abc$dollar" {
			t.Errorf("Token = %q, want the quoted token", Config.Token)
		}
		if Config.DSN != "postgres://u:p#1@localhost/db" {
			t.Errorf("DSN = %q, want the quoted DSN with its #", Config.DSN)
		}
		if Config.AISystem != "first line\nsecond line" {
			t.Errorf("AISystem = %q, want the quoted prompt with its newline", Config.AISystem)
		}
	})

	t.Run("broken values", func(t *testing.T) {
		t.Chdir(t.TempDir())
		clearZavtraEnv(t)
		Config, HTTPClientProxy = &Configuration{}, nil
		t.Setenv("ZAVTRA_TOKEN", "123:abc")
		t.Setenv("ZAVTRA_CHAT", "not-a-number")

		err := ConfigInit()
		if err == nil || !strings.Contains(err.Error(), "ZAVTRA_CHAT") {
			t.Errorf("ConfigInit: err = %v, want the error naming ZAVTRA_CHAT", err)
		}

		t.Setenv("ZAVTRA_CHAT", "-100")
		t.Setenv("ZAVTRA_ADMINS", "1,abc")
		err = ConfigInit()
		if err == nil || !strings.Contains(err.Error(), "ZAVTRA_ADMINS") {
			t.Errorf("ConfigInit (list): err = %v, want the error naming ZAVTRA_ADMINS", err)
		}

		// A file godotenv cannot parse is reported with its name.
		t.Chdir(t.TempDir())
		writeTestEnv(t, "this line has no equals sign\n")
		err = ConfigInit()
		if err == nil || !strings.Contains(err.Error(), ".env") {
			t.Errorf("ConfigInit (broken file): err = %v, want the error naming .env", err)
		}
	})

	t.Run("proxy", func(t *testing.T) {
		t.Chdir(t.TempDir())
		clearZavtraEnv(t)
		Config, HTTPClientProxy = &Configuration{}, nil
		t.Setenv("ZAVTRA_TOKEN", "123:abc")
		t.Setenv("ZAVTRA_CHAT", "-100")
		t.Setenv("ZAVTRA_PROXY", "http://127.0.0.1:8080")

		if err := ConfigInit(); err != nil {
			t.Fatalf("ConfigInit: %v", err)
		}
		if HTTPClientProxy == nil {
			t.Errorf("HTTPClientProxy = nil, want the proxy transport")
		}

		// An unparsable proxy is an error, and the transport is left alone.
		Config, HTTPClientProxy = &Configuration{}, nil
		t.Setenv("ZAVTRA_PROXY", "http://[::1")

		if err := ConfigInit(); err == nil {
			t.Errorf("ConfigInit (broken proxy): err = nil, want the URL parse error")
		}
		if HTTPClientProxy != nil {
			t.Errorf("HTTPClientProxy is set (broken proxy), want nil")
		}
	})
}
