package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// pmTexts returns the text of sendMessage calls made to a private chat.
func pmTexts(client *fakeBotClient, chatID int64) []string {
	var out []string
	for _, call := range client.callsFor("sendMessage") {
		if call.Params["chat_id"] == chatID {
			if text, ok := call.Params["text"].(string); ok {
				out = append(out, text)
			}
		}
	}
	return out
}

// TestDebug covers the admin JSON dump of a message (sent to the sender's PM).
func TestDebug(t *testing.T) {
	bot, client := setupTestEnv(t)

	// Non-admin -> Denied.
	if err := Debug(bot, testContext(bot, "/debug", testUser(42))); err != nil {
		t.Fatalf("Debug (non-admin): %v", err)
	}
	if texts := client.sentTexts(); len(texts) != 1 || texts[0] != "нет" {
		t.Errorf("Debug (non-admin) sent %q, want Nope text", texts)
	}

	// Admin: the command message is deleted, the dump goes to the PM.
	client.Calls = nil
	if err := Debug(bot, testContext(bot, "/debug", testUser(100))); err != nil {
		t.Fatalf("Debug: %v", err)
	}
	if n := len(client.callsFor("deleteMessage")); n != 1 {
		t.Errorf("Debug: deleteMessage calls = %d, want 1", n)
	}
	texts := pmTexts(client, 100)
	if len(texts) != 1 || !strings.Contains(texts[0], "<pre>") || !strings.Contains(texts[0], fmt.Sprintf("\"message_id\": %d", 100)) {
		t.Errorf("Debug sent %q, want a JSON dump of message 100", texts)
	}
	if n := len(client.callsFor("sendMessage")); n != 1 {
		t.Errorf("Debug: sendMessage calls = %d, want 1 (PM only)", n)
	}

	// Replying dumps the replied-to message instead.
	client.Calls = nil
	replyTo := testMessage("чужое сообщение", &targetUser)
	replyTo.MessageId = 55
	if err := Debug(bot, testReplyContext(bot, "/debug", testUser(100), replyTo)); err != nil {
		t.Fatalf("Debug (reply): %v", err)
	}
	texts = pmTexts(client, 100)
	if len(texts) != 1 || !strings.Contains(texts[0], "\"message_id\": 55") || !strings.Contains(texts[0], "чужое сообщение") {
		t.Errorf("Debug (reply) sent %q, want a JSON dump of message 55", texts)
	}
}

// TestDebugUser covers the admin JSON dump of a chat member.
func TestDebugUser(t *testing.T) {
	bot, client := setupTestEnv(t)

	// Non-admin -> Denied.
	if err := DebugUser(bot, testContext(bot, "/debuguser", testUser(42))); err != nil {
		t.Fatalf("DebugUser (non-admin): %v", err)
	}
	if texts := client.sentTexts(); len(texts) != 1 || texts[0] != "нет" {
		t.Errorf("DebugUser (non-admin) sent %q, want Nope text", texts)
	}

	// Replying dumps the replied-to message author.
	client.Calls = nil
	replyTo := testMessage("сообщение", &targetUser)
	if err := DebugUser(bot, testReplyContext(bot, "/debuguser", testUser(100), replyTo)); err != nil {
		t.Fatalf("DebugUser (reply): %v", err)
	}
	calls := client.callsFor("getChatMember")
	if len(calls) != 1 || calls[0].Params["user_id"] != int64(777) {
		t.Fatalf("DebugUser (reply): getChatMember calls = %v, want one for user 777", calls)
	}
	texts := pmTexts(client, 100)
	if len(texts) != 1 || !strings.Contains(texts[0], "\"status\": \"member\"") || !strings.Contains(texts[0], "\"id\": 777") {
		t.Errorf("DebugUser (reply) sent %q, want the merged chat member JSON", texts)
	}

	// No reply and no resolvable argument -> the lookup error is returned.
	client.Calls = nil
	if err := DebugUser(bot, testContext(bot, "/debuguser", testUser(100))); err == nil {
		t.Error("DebugUser (no target): expected an error, got nil")
	}
	if len(pmTexts(client, 100)) != 0 {
		t.Errorf("DebugUser (no target): unexpected PM message: %q", pmTexts(client, 100))
	}

	// An ID argument resolves through the users table.
	client.Calls = nil
	seedUser(777, "target", "Цель")
	if err := DebugUser(bot, testContext(bot, "/debuguser 777", testUser(100))); err != nil {
		t.Fatalf("DebugUser (arg): %v", err)
	}
	if calls := client.callsFor("getChatMember"); len(calls) != 1 || calls[0].Params["user_id"] != int64(777) {
		t.Errorf("DebugUser (arg): getChatMember calls = %v, want one for user 777", calls)
	}
}

// TestFindUserInMessageTest covers the admin user lookup debug command.
func TestFindUserInMessageTest(t *testing.T) {
	bot, client := setupTestEnv(t)

	// Non-admin -> Denied.
	if err := FindUserInMessageTest(bot, testContext(bot, "/finduserinmessage", testUser(42))); err != nil {
		t.Fatalf("FindUserInMessageTest (non-admin): %v", err)
	}
	if texts := client.sentTexts(); len(texts) != 1 || texts[0] != "нет" {
		t.Errorf("FindUserInMessageTest (non-admin) sent %q, want Nope text", texts)
	}

	// Usage message when not replying.
	client.Calls = nil
	if err := FindUserInMessageTest(bot, testContext(bot, "/finduserinmessage", testUser(100))); err != nil {
		t.Fatalf("FindUserInMessageTest (usage): %v", err)
	}
	texts := client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "Укажите сообщение") {
		t.Errorf("FindUserInMessageTest (usage) sent %q, want usage message", texts)
	}

	// Replying looks the author up in the replied-to message: a text_mention
	// entity is the reliable way to identify a user there.
	client.Calls = nil
	replyTo := testMessage("упомянутый человек", &targetUser)
	replyTo.Entities = []gotgbot.MessageEntity{{Type: "text_mention", Offset: 0, Length: 9, User: &targetUser}}
	if err := FindUserInMessageTest(bot, testReplyContext(bot, "/finduserinmessage", testUser(100), replyTo)); err != nil {
		t.Fatalf("FindUserInMessageTest (reply): %v", err)
	}
	texts = client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "ID: 777") || !strings.Contains(texts[0], "Username: target") {
		t.Errorf("FindUserInMessageTest (reply) sent %q, want the found user details", texts)
	}

	// A replied-to message without any identifiable user reports the error.
	client.Calls = nil
	anonymous := testMessage("просто текст", &targetUser)
	if err := FindUserInMessageTest(bot, testReplyContext(bot, "/finduserinmessage", testUser(100), anonymous)); err == nil {
		t.Error("FindUserInMessageTest (no user in reply): expected an error, got nil")
	}
}

// TestTestRandom covers the admin RNG distribution report command (the handler
// itself is named TestRandom, so the test name needs an extra prefix).
func TestTestRandom(t *testing.T) {
	bot, client := setupTestEnv(t)

	// Non-admin -> Denied.
	if err := TestRandom(bot, testContext(bot, "/testrandom", testUser(42))); err != nil {
		t.Fatalf("TestRandom (non-admin): %v", err)
	}
	if texts := client.sentTexts(); len(texts) != 1 || texts[0] != "нет" {
		t.Errorf("TestRandom (non-admin) sent %q, want Nope text", texts)
	}

	// Admin: 1000 draws must all land in the requested 0..9 range.
	client.Calls = nil
	if err := TestRandom(bot, testContext(bot, "/testrandom", testUser(100))); err != nil {
		t.Fatalf("TestRandom: %v", err)
	}
	texts := client.sentTexts()
	if len(texts) != 1 || !strings.HasPrefix(texts[0], "1000xRandInt(0, 9):") {
		t.Fatalf("TestRandom sent %q, want the distribution report", texts)
	}
	total := 0
	for _, line := range strings.Split(strings.TrimSpace(texts[0]), "\n")[1:] {
		var bucket, count int
		if _, err := fmt.Sscanf(line, "%d - %d", &bucket, &count); err != nil {
			t.Fatalf("TestRandom: unparsable report line %q: %v", line, err)
		}
		if bucket < 0 || bucket > 9 {
			t.Errorf("TestRandom: bucket %d outside the requested 0..9 range", bucket)
		}
		if count == 0 {
			t.Errorf("TestRandom: bucket %d was never hit in 1000 draws", bucket)
		}
		total += count
	}
	if total != 1000 {
		t.Errorf("TestRandom: total draws = %d, want 1000", total)
	}
}
