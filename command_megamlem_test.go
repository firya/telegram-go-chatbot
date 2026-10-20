package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

func TestMegamlem(t *testing.T) {
	for _, tc := range []struct {
		name    string
		userID  int64
		allowed bool
	}{
		{"member", 42, false},
		{"admin", 100, true},
		{"moderator", 300, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bot, client := setupTestEnv(t)
			for id := int64(701); id <= 714; id++ {
				seedChatStats(id)
				DB.Model(&Stats{}).Where("context_id = ?", id).Update("last_update", id)
			}
			client.setMemberStatus(714, "administrator")
			client.setMemberStatus(713, "restricted")

			// Invoke the registered command, including Telegram's @bot suffix.
			registered := false
			for _, entry := range commandListConfig() {
				if entry.command.Command == "megamlem" {
					registered = true
					if err := entry.response(bot, testContext(bot, "/megamlem@testbot", testUser(tc.userID))); err != nil {
						t.Fatal(err)
					}
				}
			}
			if !registered {
				t.Fatal("/megamlem is not registered")
			}
			if !tc.allowed {
				if texts := client.sentTexts(); len(texts) != 1 || texts[0] != "нет" {
					t.Fatalf("denied response = %q", texts)
				}
				for _, method := range []string{"deleteMessage", "getChatMember", "restrictChatMember", "sendPhoto"} {
					if len(client.callsFor(method)) != 0 {
						t.Errorf("unauthorized user triggered %s", method)
					}
				}
				return
			}

			if len(client.callsFor("deleteMessage")) != 1 {
				t.Error("command message was not removed")
			}
			restrictions := client.callsFor("restrictChatMember")
			if len(restrictions) != 10 {
				t.Fatalf("restricted %d users, want 10", len(restrictions))
			}
			for i, call := range restrictions {
				id := int64(712 - i)
				if call.Params["user_id"] != id || call.Params["chat_id"] != int64(-100) {
					t.Errorf("restriction %d = %v, want recent member %d in the same chat", i, call.Params, id)
				}
				permissions := call.Params["permissions"].(gotgbot.ChatPermissions)
				if permissions.CanSendMessages || permissions.CanReactToMessages == nil || !*permissions.CanReactToMessages {
					t.Errorf("unexpected restriction permissions: %+v", permissions)
				}
				remaining := call.Params["until_date"].(int64) - time.Now().Unix()
				if remaining < 55 || remaining > 60 {
					t.Errorf("mute duration = %d, want 60 seconds", remaining)
				}
				if duelistDeaths(t, id) != 1 || statTotal(t, 8, id) != 1 || muteSeconds(t, id) != 60 {
					t.Errorf("member %d did not receive the same game stats as /shotgun", id)
				}
			}
			photos := client.callsFor("sendPhoto")
			if len(photos) != 1 {
				t.Fatalf("sent %d photos, want 1", len(photos))
			}
			photo := photos[0]
			caption, _ := photo.Params["caption"].(string)
			if !strings.HasPrefix(caption, "Неизвестно откуда появившийся гигантский чёрный кот совершает <b>МЕГАМЛЕМ!</b>\n\n") ||
				!strings.Contains(caption, "member да.") || !strings.HasSuffix(caption, "Респавн через пару минут.") {
				t.Errorf("unexpected photo caption: %q", caption)
			}
			if photo.Params["parse_mode"] != gotgbot.ParseModeHTML || photo.Params["disable_notification"] != true || photo.Params["chat_id"] != int64(-100) {
				t.Errorf("unexpected photo options: %v", photo.Params)
			}
			file, ok := photo.Params["photo"].(*gotgbot.FileReader)
			if !ok {
				t.Fatalf("photo = %T, want the embedded attachment", photo.Params["photo"])
			}
			data, err := io.ReadAll(file.Data)
			if err != nil || !bytes.Equal(data, megamlemJPG) || len(data) == 0 {
				t.Errorf("photo does not contain the attached image: %v", err)
			}
			if len(client.sentTexts()) != 0 {
				t.Error("normal result should fit in the photo caption")
			}
		})
	}
}

func TestMegamlemPremium(t *testing.T) {
	bot, client := setupTestEnv(t)
	seedChatStats(777)
	client.setResult("getChatMember", json.RawMessage(`{"status":"member","user":{"id":777,"first_name":"Premium","is_premium":true}}`))
	if err := Shotgun(bot, testContext(bot, "/megamlem", testUser(100))); err != nil {
		t.Fatal(err)
	}
	if got := muteSeconds(t, 777); got != 120 {
		t.Errorf("premium mute = %d seconds, want 120", got)
	}
	restrictions := client.callsFor("restrictChatMember")
	if len(restrictions) != 1 {
		t.Fatalf("premium restrictions = %d, want 1", len(restrictions))
	}
	remaining := restrictions[0].Params["until_date"].(int64) - time.Now().Unix()
	if remaining < 115 || remaining > 120 {
		t.Errorf("premium restriction lasts %d seconds, want 120", remaining)
	}
	if caption := client.callsFor("sendPhoto")[0].Params["caption"].(string); !strings.Contains(caption, "Premium премиально да.") {
		t.Errorf("missing premium result in %q", caption)
	}
}

func TestMegamlemLongResult(t *testing.T) {
	bot, client := setupTestEnv(t)
	seedChatStats(777)
	DB.Model(&Bless{}).Where("text = ?", "да").Update("text", strings.Repeat("Млем ", 250))
	if err := Shotgun(bot, testContext(bot, "/megamlem", testUser(100))); err != nil {
		t.Fatal(err)
	}
	photos := client.callsFor("sendPhoto")
	if len(photos) != 1 || photos[0].Params["caption"] != "Неизвестно откуда появившийся гигантский чёрный кот совершает <b>МЕГАМЛЕМ!</b>" {
		t.Fatalf("long result did not retain the picture and headline: %v", photos)
	}
	texts := client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], strings.Repeat("Млем ", 250)) || !strings.HasSuffix(texts[0], "Респавн через пару минут.") {
		t.Errorf("long result was lost or truncated: %q", texts)
	}
}

func TestMegamlemPhotoFailure(t *testing.T) {
	bot, client := setupTestEnv(t)
	wantErr := errors.New("photo upload failed")
	client.setError("sendPhoto", wantErr)
	if err := Shotgun(bot, testContext(bot, "/megamlem", testUser(100))); !errors.Is(err, wantErr) {
		t.Errorf("error = %v, want upload failure", err)
	}
}
