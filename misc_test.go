package main

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

// TestGoogle covers the search link builder (it performs no HTTP request).
func TestGoogle(t *testing.T) {
	bot, client := setupTestEnv(t)

	// Usage message without a query.
	if err := Google(bot, testContext(bot, "/google", testUser(42))); err != nil {
		t.Fatalf("Google (usage): %v", err)
	}
	texts := client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "/google {запрос}") {
		t.Errorf("Google (usage) sent %q, want usage message", texts)
	}

	// The query is lowercased and URL escaped.
	client.Calls = nil
	if err := Google(bot, testContext(bot, "/google Привет Мир", testUser(42))); err != nil {
		t.Fatalf("Google: %v", err)
	}
	want := "https://www.google.com/search?q=" + url.QueryEscape("привет мир")
	texts = client.sentTexts()
	if len(texts) != 1 || texts[0] != want {
		t.Errorf("Google sent %q, want %q", texts, want)
	}

	// Replying anchors the link to the replied-to message instead.
	client.Calls = nil
	replyTo := testMessage("запрос", &targetUser)
	replyTo.MessageId = 55
	if err := Google(bot, testReplyContext(bot, "/google что поискать", testUser(42), replyTo)); err != nil {
		t.Fatalf("Google (reply): %v", err)
	}
	calls := client.callsFor("sendMessage")
	if len(calls) != 1 {
		t.Fatalf("Google (reply): sendMessage calls = %d, want 1", len(calls))
	}
	params, ok := calls[0].Params["reply_parameters"].(*gotgbot.ReplyParameters)
	if !ok {
		t.Fatalf("Google (reply): reply_parameters param missing: %v", calls[0].Params["reply_parameters"])
	}
	if params.MessageId != replyTo.MessageId || !params.AllowSendingWithoutReply {
		t.Errorf("Google (reply): reply_parameters = %+v, want message_id %d with AllowSendingWithoutReply", params, replyTo.MessageId)
	}
}

// TestDownload covers the parts of the yt-dlp handler that do not need the
// yt-dlp binary: usage, the "no links found" error, and the reaction handling.
func TestDownload(t *testing.T) {
	bot, client := setupTestEnv(t)

	// Usage message without a link or a reply.
	if err := Download(bot, testContext(bot, "/download", testUser(42))); err != nil {
		t.Fatalf("Download (usage): %v", err)
	}
	texts := client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "/download {ссылка") {
		t.Errorf("Download (usage) sent %q, want usage message", texts)
	}

	// A plain-text URL is not an entity, so no link is found and the message
	// gets the "watching" reaction.
	client.Calls = nil
	err := Download(bot, testContext(bot, "/download https://www.youtube.com/watch?v=123", testUser(42)))
	if err == nil || !strings.Contains(err.Error(), "ссылок не найдено") {
		t.Errorf("Download (no entities): err = %v, want the missing link error", err)
	}
	if n := len(client.callsFor("setMessageReaction")); n != 1 {
		t.Errorf("Download (no entities): setMessageReaction calls = %d, want 1", n)
	}
	if n := len(client.callsFor("deleteMessage")); n != 0 {
		t.Errorf("Download (no entities): deleteMessage calls = %d, want 0", n)
	}

	// The " remove" variant deletes the command instead of reacting.
	client.Calls = nil
	err = Download(bot, testContext(bot, "/download remove", testUser(42)))
	if err == nil {
		t.Error("Download (remove): expected the missing link error, got nil")
	}
	if n := len(client.callsFor("deleteMessage")); n != 1 {
		t.Errorf("Download (remove): deleteMessage calls = %d, want 1", n)
	}
	if n := len(client.callsFor("setMessageReaction")); n != 0 {
		t.Errorf("Download (remove): setMessageReaction calls = %d, want 0", n)
	}
}

// TestOnReaction covers the reaction -> stats counter handler.
func TestOnReaction(t *testing.T) {
	bot, _ := setupTestEnv(t)

	reactionUpdate := func(newReaction []gotgbot.ReactionType) *ext.Context {
		return ext.NewContext(bot, &gotgbot.Update{UpdateId: 1, MessageReaction: &gotgbot.MessageReactionUpdated{
			Chat:        gotgbot.Chat{Id: Config.Chat, Type: "group"},
			MessageId:   55,
			Date:        time.Now().Unix(),
			NewReaction: newReaction,
		}}, nil)
	}

	// An update without a reaction is ignored.
	if err := OnReaction(bot, ext.NewContext(bot, &gotgbot.Update{UpdateId: 1}, nil)); err != nil {
		t.Fatalf("OnReaction (no reaction): %v", err)
	}
	// Removing a reaction (empty NewReaction) is ignored too.
	if err := OnReaction(bot, reactionUpdate(nil)); err != nil {
		t.Fatalf("OnReaction (removed reaction): %v", err)
	}
	var stats Stats
	if err := DB.First(&stats, "stat_type = ? AND context_id = ?", 6, 55).Error; err == nil {
		t.Errorf("OnReaction: stats row created for an ignored reaction: %+v", stats)
	}

	// A real reaction counts once per message.
	if err := OnReaction(bot, reactionUpdate([]gotgbot.ReactionType{gotgbot.ReactionTypeEmoji{Emoji: "👍"}})); err != nil {
		t.Fatalf("OnReaction: %v", err)
	}
	var reactionStats Stats
	if err := DB.First(&reactionStats, "stat_type = ? AND context_id = ?", 6, 55).Error; err != nil {
		t.Fatalf("OnReaction: stats row not created: %v", err)
	}
	if reactionStats.Count != 1 {
		t.Errorf("OnReaction: stats count = %d, want 1", reactionStats.Count)
	}
	if err := OnReaction(bot, reactionUpdate([]gotgbot.ReactionType{gotgbot.ReactionTypeEmoji{Emoji: "👍"}})); err != nil {
		t.Fatalf("OnReaction (second): %v", err)
	}
	var secondStats Stats
	if err := DB.First(&secondStats, "stat_type = ? AND context_id = ?", 6, 55).Error; err != nil {
		t.Fatalf("OnReaction (second): stats row lost: %v", err)
	}
	if secondStats.Count != 2 {
		t.Errorf("OnReaction (second): stats count = %d, want 2", secondStats.Count)
	}
}

// TestRemoveJoinMessageAndJoinUser covers cleaning up join messages in the
// reserve chat and unbanning everyone who joined.
func TestRemoveJoinMessageAndJoinUser(t *testing.T) {
	bot, client := setupTestEnv(t)

	msg := testMessage("вошли в чат", testUser(42))
	msg.Chat = gotgbot.Chat{Id: Config.ReserveChat, Type: "supergroup"}
	msg.NewChatMembers = []gotgbot.User{{Id: 777, FirstName: "Первый"}, {Id: 778, FirstName: "Второй"}}
	if err := RemoveJoinMessageAndJoinUser(bot, ext.NewContext(bot, &gotgbot.Update{UpdateId: 1, Message: msg}, nil)); err != nil {
		t.Fatalf("RemoveJoinMessageAndJoinUser: %v", err)
	}
	if n := len(client.callsFor("deleteMessage")); n != 1 {
		t.Errorf("RemoveJoinMessageAndJoinUser: deleteMessage calls = %d, want 1", n)
	}
	calls := client.callsFor("unbanChatMember")
	if len(calls) != 2 {
		t.Fatalf("RemoveJoinMessageAndJoinUser: unbanChatMember calls = %d, want 2", len(calls))
	}
	for i, want := range []int64{777, 778} {
		if calls[i].Params["user_id"] != want {
			t.Errorf("RemoveJoinMessageAndJoinUser: unban %d user_id = %v, want %d", i, calls[i].Params["user_id"], want)
		}
	}
}
