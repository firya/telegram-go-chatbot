package main

import (
	"errors"
	"testing"
)

func TestKhaleesiReplyContent(t *testing.T) {
	cases := []struct {
		name, text, caption, want string
		removed                   bool
	}{
		{"caption", "", "холодильник", "хойодийник", true},
		{"text before caption", "вещи", "зонтик", "веси", true},
		{"literal markup", "<b>что</b> & 1 < 2 😀", "", "<b>стё</b> & 1 < 2 😀", true},
		{"no text", "", "", "Текст не найден в указанном сообщении.", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bot, client := setupTestEnv(t)
			target := testMessage(c.text, &targetUser)
			target.Caption = c.caption
			ctx := testReplyContext(bot, "/khaleesi", testUser(42), target)
			if err := Khaleesi(bot, ctx); err != nil {
				t.Fatal(err)
			}
			texts := client.sentTexts()
			if len(texts) != 1 || texts[0] != c.want {
				t.Fatalf("sent %q, want [%q]", texts, c.want)
			}
			if c.removed {
				call := client.callsFor("sendMessage")[0]
				if mode := call.Params["parse_mode"]; mode != nil && mode != "" {
					t.Errorf("reply must be plain text, parse_mode = %v", mode)
				}
			}
			if removed := len(client.callsFor("deleteMessage")) != 0; removed != c.removed {
				t.Errorf("command removed = %v, want %v", removed, c.removed)
			}
		})
	}
}

func TestKhaleesiSendFailureKeepsCommand(t *testing.T) {
	bot, client := setupTestEnv(t)
	wantErr := errors.New("send failed")
	client.setError("sendMessage", wantErr)
	target := testMessage("что", &targetUser)
	err := Khaleesi(bot, testReplyContext(bot, "/khaleesi", testUser(42), target))
	if !errors.Is(err, wantErr) {
		t.Errorf("error = %v, want %v", err, wantErr)
	}
	if len(client.callsFor("deleteMessage")) != 0 {
		t.Error("command was removed despite send failure")
	}
}
