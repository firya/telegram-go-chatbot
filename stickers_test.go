package main

import (
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// stickerReplyId reads the reply_to_message_id from a recorded call's
// "reply_parameters" param (gotgbot passes ReplyParameters as a struct).
func stickerReplyId(t *testing.T, call fakeCall) int64 {
	t.Helper()
	params, ok := call.Params["reply_parameters"].(*gotgbot.ReplyParameters)
	if !ok {
		t.Errorf("%v: reply_parameters param missing: %v", call.Method, call.Params["reply_parameters"])
		return 0
	}
	return params.MessageId
}

// TestBonk covers the "write the sender's name on the bonk picture" handler.
func TestBonk(t *testing.T) {
	bot, client := setupTestEnv(t)

	// Usage message when not replying.
	if err := Bonk(bot, testContext(bot, "/bonk", testUser(42))); err != nil {
		t.Fatalf("Bonk (usage): %v", err)
	}
	texts := client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "/bonk") {
		t.Errorf("Bonk (usage) sent %q, want usage message", texts)
	}
	if n := len(client.callsFor("sendSticker")); n != 0 {
		t.Errorf("Bonk (usage): sendSticker calls = %d, want 0", n)
	}

	// Replying draws on the embedded picture and sends it as a sticker.
	client.Calls = nil
	replyTo := testMessage("сообщение", &targetUser)
	if err := Bonk(bot, testReplyContext(bot, "/bonk", testUser(42), replyTo)); err != nil {
		t.Fatalf("Bonk: %v", err)
	}
	if n := len(client.callsFor("deleteMessage")); n != 1 {
		t.Errorf("Bonk: deleteMessage calls = %d, want 1", n)
	}
	calls := client.callsFor("sendSticker")
	if len(calls) != 1 {
		t.Fatalf("Bonk: sendSticker calls = %d, want 1", len(calls))
	}
	if calls[0].Params["chat_id"] != int64(Config.Chat) {
		t.Errorf("Bonk: chat_id = %v, want %v", calls[0].Params["chat_id"], Config.Chat)
	}
	if id := stickerReplyId(t, calls[0]); id != replyTo.MessageId {
		t.Errorf("Bonk: reply_to_message_id = %d, want %d", id, replyTo.MessageId)
	}
	if _, ok := calls[0].Params["sticker"].(*gotgbot.FileReader); !ok {
		t.Errorf("Bonk: sticker param = %T, want *gotgbot.FileReader (rendered PNG)", calls[0].Params["sticker"])
	}
}

// TestHug covers the "write the sender's name on the hug picture" handler.
func TestHug(t *testing.T) {
	bot, client := setupTestEnv(t)

	// Usage message when not replying.
	if err := Hug(bot, testContext(bot, "/hug", testUser(42))); err != nil {
		t.Fatalf("Hug (usage): %v", err)
	}
	texts := client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "/hug") {
		t.Errorf("Hug (usage) sent %q, want usage message", texts)
	}

	client.Calls = nil
	replyTo := testMessage("сообщение", &targetUser)
	if err := Hug(bot, testReplyContext(bot, "/hug", testUser(42), replyTo)); err != nil {
		t.Fatalf("Hug: %v", err)
	}
	if n := len(client.callsFor("deleteMessage")); n != 1 {
		t.Errorf("Hug: deleteMessage calls = %d, want 1", n)
	}
	calls := client.callsFor("sendSticker")
	if len(calls) != 1 {
		t.Fatalf("Hug: sendSticker calls = %d, want 1", len(calls))
	}
	if id := stickerReplyId(t, calls[0]); id != replyTo.MessageId {
		t.Errorf("Hug: reply_to_message_id = %d, want %d", id, replyTo.MessageId)
	}
	if _, ok := calls[0].Params["sticker"].(*gotgbot.FileReader); !ok {
		t.Errorf("Hug: sticker param = %T, want *gotgbot.FileReader (rendered PNG)", calls[0].Params["sticker"])
	}
}
