package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// writeTestConfig drops a config.json into the test working directory.
func writeTestConfig(t *testing.T, contents string) {
	t.Helper()

	if err := os.WriteFile("config.json", []byte(contents), 0o600); err != nil {
		t.Fatalf("write config.json: %v", err)
	}
}

// TestConfigInit covers config.json handling: the file is created with defaults
// when it is missing, the mandatory fields are enforced, the release calendar
// gets its default, a proxy switches the transport, and a broken file is
// reported. Every case runs in its own directory, so the real config.json of
// the checkout is never touched.
func TestConfigInit(t *testing.T) {
	oldConfig, oldProxy := Config, HTTPClientProxy
	t.Cleanup(func() { Config, HTTPClientProxy = oldConfig, oldProxy })

	t.Run("created with defaults", func(t *testing.T) {
		t.Chdir(t.TempDir())
		Config, HTTPClientProxy = &Configuration{}, nil

		err := ConfigInit()
		if err == nil || !strings.Contains(err.Error(), "sting 'token' not found") {
			t.Errorf("ConfigInit: err = %v, want the missing token error", err)
		}

		data, err := os.ReadFile("config.json")
		if err != nil {
			t.Fatalf("ConfigInit wrote no config.json: %v", err)
		}
		var written Configuration
		if err := json.Unmarshal(data, &written); err != nil {
			t.Fatalf("config.json is not valid JSON: %v", err)
		}
		if written.BotApiUrl != "https://api.telegram.org" {
			t.Errorf("BotApiUrl = %q, want the default Bot API URL", written.BotApiUrl)
		}
		if len(written.AllowedUpdates) != 4 {
			t.Errorf("AllowedUpdates = %v, want the four default update types", written.AllowedUpdates)
		}
		if len(written.Admins) != 0 || len(written.Moders) != 0 {
			t.Errorf("Admins/Moders = %v/%v, want empty lists", written.Admins, written.Moders)
		}
		if info, err := os.Stat("config.json"); err != nil || info.Mode().Perm() != 0o600 {
			t.Errorf("config.json mode = %v, want 0600", info)
		}
		if Config.ReleasesUrl != DefaultReleasesUrl {
			t.Errorf("ReleasesUrl = %q, want the default release calendar", Config.ReleasesUrl)
		}
	})

	t.Run("token and chat are mandatory", func(t *testing.T) {
		t.Chdir(t.TempDir())
		Config, HTTPClientProxy = &Configuration{}, nil
		writeTestConfig(t, `{"token":"123:abc"}`)

		err := ConfigInit()
		if err == nil || !strings.Contains(err.Error(), "integer 'chat' not found") {
			t.Errorf("ConfigInit: err = %v, want the missing chat error", err)
		}

		// Decoding reuses the Config that is already set, so the previous
		// values survive a re-read: the token has to be gone from the config
		// object itself and from the struct.
		Config = &Configuration{}
		writeTestConfig(t, `{"chat":-100}`)
		err = ConfigInit()
		if err == nil || !strings.Contains(err.Error(), "sting 'token' not found") {
			t.Errorf("ConfigInit: err = %v, want the missing token error", err)
		}
	})

	t.Run("broken config.json", func(t *testing.T) {
		t.Chdir(t.TempDir())
		Config, HTTPClientProxy = &Configuration{}, nil
		writeTestConfig(t, `{"token":`)

		if err := ConfigInit(); err == nil {
			t.Errorf("ConfigInit: err = nil, want the JSON decode error")
		}
	})

	t.Run("loaded values", func(t *testing.T) {
		t.Chdir(t.TempDir())
		Config, HTTPClientProxy = &Configuration{}, nil
		writeTestConfig(t, `{"token":"123:abc","chat":-100,"reserve_chat":-200,"sysadmin":7,`+
			`"admins":[1,2],"moders":[3],"releases_url":"https://example.invalid/cal",`+
			`"antispam_window":15,"antispam_count":4}`)

		if err := ConfigInit(); err != nil {
			t.Fatalf("ConfigInit: %v", err)
		}
		if Config.Chat != -100 || Config.ReserveChat != -200 || Config.SysAdmin != 7 {
			t.Errorf("chats = %v/%v, sysadmin = %v, want the values from the file",
				Config.Chat, Config.ReserveChat, Config.SysAdmin)
		}
		if len(Config.Admins) != 2 || len(Config.Moders) != 1 {
			t.Errorf("Admins/Moders = %v/%v, want the lists from the file", Config.Admins, Config.Moders)
		}
		if Config.AntiSpamWindow != 15 || Config.AntiSpamCount != 4 {
			t.Errorf("antispam = %v/%v, want 15/4", Config.AntiSpamWindow, Config.AntiSpamCount)
		}
		if Config.ReleasesUrl != "https://example.invalid/cal" {
			t.Errorf("ReleasesUrl = %q, want the configured calendar", Config.ReleasesUrl)
		}
		if HTTPClientProxy != nil {
			t.Errorf("HTTPClientProxy set = %v, want no proxy transport without a proxy", HTTPClientProxy != nil)
		}
	})

	t.Run("proxy", func(t *testing.T) {
		t.Chdir(t.TempDir())
		Config, HTTPClientProxy = &Configuration{}, nil
		writeTestConfig(t, `{"token":"123:abc","chat":-100,"proxy":"http://127.0.0.1:8080"}`)

		if err := ConfigInit(); err != nil {
			t.Fatalf("ConfigInit: %v", err)
		}
		if HTTPClientProxy == nil {
			t.Errorf("HTTPClientProxy = nil, want the proxy transport")
		}

		// An unparsable proxy is an error, and the transport is left alone.
		Config, HTTPClientProxy = &Configuration{}, nil
		writeTestConfig(t, `{"token":"123:abc","chat":-100,"proxy":"http://[::1"}`)

		if err := ConfigInit(); err == nil {
			t.Errorf("ConfigInit (broken proxy): err = nil, want the URL parse error")
		}
		if HTTPClientProxy != nil {
			t.Errorf("HTTPClientProxy set (broken proxy) = %v, want nil", HTTPClientProxy != nil)
		}
	})
}
