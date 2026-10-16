package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

// TestOnText covers the global text handler: stats recording, user upsert and
// the reserve-chat auto-unban.
func TestOnText(t *testing.T) {
	bot, client := setupTestEnv(t)

	// Regular chat: user upserted, activity stats recorded, nothing deleted.
	if err := OnText(bot, testContext(bot, "привет мир", testUser(42))); err != nil {
		t.Fatalf("OnText: %v", err)
	}
	time.Sleep(200 * time.Millisecond) // let the CheckUserBan goroutine finish
	if n := len(client.callsFor("deleteMessage")); n != 0 {
		t.Errorf("OnText: deleteMessage calls = %d, want 0", n)
	}
	var user gotgbot.User
	if err := DB.First(&user, 42).Error; err != nil {
		t.Errorf("OnText: user not upserted: %v", err)
	}
	var stats Stats
	if err := DB.First(&stats, "stat_type = ? AND context_id = ?", 3, 42).Error; err != nil || stats.Count != 1 {
		t.Errorf("OnText: user stats = %+v (err %v), want count 1", stats, err)
	}
	var words int64
	DB.Model(&StatsWords{}).Count(&words)
	if words < 2 {
		t.Errorf("OnText: word stats = %d, want at least 2 (привет, мир)", words)
	}

	// Commands do not feed the word stats but do bump activity.
	client.Calls = nil
	if err := OnText(bot, testContext(bot, "/ping", testUser(42))); err != nil {
		t.Fatalf("OnText (command): %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	var wordsAfter int64
	DB.Model(&StatsWords{}).Count(&wordsAfter)
	if wordsAfter != words {
		t.Errorf("OnText (command): word stats grew from %d to %d", words, wordsAfter)
	}
	DB.First(&stats, "stat_type = ? AND context_id = ?", 3, 42)
	if stats.Count != 2 {
		t.Errorf("OnText (command): user stats count = %d, want 2", stats.Count)
	}

	// Reserve chat: messages are deleted and non-admins are auto-unbanned.
	client.Calls = nil
	reserve := testMessage("привет", testUser(42))
	reserve.Chat = gotgbot.Chat{Id: Config.ReserveChat, Type: "supergroup"}
	if err := OnText(bot, ext.NewContext(bot, &gotgbot.Update{UpdateId: 1, Message: reserve}, nil)); err != nil {
		t.Fatalf("OnText (reserve): %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if n := len(client.callsFor("deleteMessage")); n != 1 {
		t.Errorf("OnText (reserve): deleteMessage calls = %d, want 1", n)
	}
	if n := len(client.callsFor("unbanChatMember")); n != 1 {
		t.Errorf("OnText (reserve): unbanChatMember calls = %d, want 1", n)
	}

	// Reserve chat: admins are not unbanned.
	client.Calls = nil
	adminMsg := testMessage("привет", testUser(100))
	adminMsg.Chat = gotgbot.Chat{Id: Config.ReserveChat, Type: "supergroup"}
	if err := OnText(bot, ext.NewContext(bot, &gotgbot.Update{UpdateId: 1, Message: adminMsg}, nil)); err != nil {
		t.Fatalf("OnText (reserve admin): %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if n := len(client.callsFor("unbanChatMember")); n != 0 {
		t.Errorf("OnText (reserve admin): unbanChatMember calls = %d, want 0", n)
	}
}

// TestCheckUserBan covers the "user cannot reply" warnings spawned by OnText.
func TestCheckUserBan(t *testing.T) {
	bot, client := setupTestEnv(t)

	// Left the chat.
	replyTo := testMessage("сообщение", &targetUser)
	client.MemberStatus = map[int64]string{777: "left"}
	if err := CheckUserBan(bot, testReplyContext(bot, "/hello", testUser(42), replyTo)); err != nil {
		t.Fatalf("CheckUserBan (left): %v", err)
	}
	texts := client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "не сможет ответить, т.к. вышел из чата") {
		t.Errorf("CheckUserBan (left) sent %q, want left warning", texts)
	}

	// Kicked.
	client.Calls = nil
	client.MemberStatus = map[int64]string{777: "kicked"}
	if err := CheckUserBan(bot, testReplyContext(bot, "/hello", testUser(42), replyTo)); err != nil {
		t.Fatalf("CheckUserBan (kicked): %v", err)
	}
	texts = client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "не сможет ответить, т.к. был забанен") {
		t.Errorf("CheckUserBan (kicked) sent %q, want kicked warning", texts)
	}

	// Restricted (muted).
	client.Calls = nil
	client.MemberStatus = map[int64]string{777: "restricted"}
	if err := CheckUserBan(bot, testReplyContext(bot, "/hello", testUser(42), replyTo)); err != nil {
		t.Fatalf("CheckUserBan (restricted): %v", err)
	}
	texts = client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "умир") {
		t.Errorf("CheckUserBan (restricted) sent %q, want muted warning", texts)
	}

	// Regular member: no warning.
	client.Calls = nil
	client.MemberStatus = nil
	if err := CheckUserBan(bot, testReplyContext(bot, "/hello", testUser(42), replyTo)); err != nil {
		t.Fatalf("CheckUserBan (member): %v", err)
	}
	if texts := client.sentTexts(); len(texts) != 0 {
		t.Errorf("CheckUserBan (member) sent %q, want nothing", texts)
	}
}

// TestOnChatMember covers the chat member join/leave handler.
func TestOnChatMember(t *testing.T) {
	bot, client := setupTestEnv(t)

	// Reserve chat joins are auto-unbanned and the user is upserted.
	cmu := gotgbot.ChatMemberUpdated{
		Chat:          gotgbot.Chat{Id: Config.ReserveChat, Type: "supergroup"},
		From:          *testUser(777),
		Date:          time.Now().Unix(),
		NewChatMember: gotgbot.ChatMemberMember{User: *testUser(777)},
	}
	if err := OnChatMember(bot, ext.NewContext(bot, &gotgbot.Update{UpdateId: 1, ChatMember: &cmu}, nil)); err != nil {
		t.Fatalf("OnChatMember (reserve): %v", err)
	}
	if n := len(client.callsFor("unbanChatMember")); n != 1 {
		t.Errorf("OnChatMember (reserve): unbanChatMember calls = %d, want 1", n)
	}
	var joinedUser gotgbot.User
	if err := DB.First(&joinedUser, 777).Error; err != nil {
		t.Errorf("OnChatMember (reserve): user not upserted: %v", err)
	}

	// Main chat joins are only recorded, not unbanned.
	client.Calls = nil
	cmu.Chat = gotgbot.Chat{Id: Config.Chat, Type: "supergroup"}
	cmu.From = *testUser(778)
	cmu.NewChatMember = gotgbot.ChatMemberMember{User: *testUser(778)}
	if err := OnChatMember(bot, ext.NewContext(bot, &gotgbot.Update{UpdateId: 1, ChatMember: &cmu}, nil)); err != nil {
		t.Fatalf("OnChatMember: %v", err)
	}
	if n := len(client.callsFor("unbanChatMember")); n != 0 {
		t.Errorf("OnChatMember: unbanChatMember calls = %d, want 0", n)
	}
	var mainChatUser gotgbot.User
	if err := DB.First(&mainChatUser, 778).Error; err != nil {
		t.Errorf("OnChatMember: user not upserted: %v", err)
	}
}

// TestSaveToPM covers copying a replied-to message into the sender's PM.
func TestSaveToPM(t *testing.T) {
	bot, client := setupTestEnv(t)

	// Usage message when not replying.
	if err := SaveToPM(bot, testContext(bot, "/topm", testUser(42))); err != nil {
		t.Fatalf("SaveToPM (usage): %v", err)
	}
	texts := client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "Пример использования") {
		t.Errorf("SaveToPM (usage) sent %q, want usage message", texts)
	}

	// Single message: copied to the sender's PM with a link, command deleted.
	client.Calls = nil
	target := testMessage("пост", testUser(777))
	target.MessageId = 55
	msg := testMessage("/topm", testUser(42))
	msg.Chat = gotgbot.Chat{Id: -100123, Type: "channel"}
	msg.ReplyToMessage = target
	if err := SaveToPM(bot, ext.NewContext(bot, &gotgbot.Update{UpdateId: 1, Message: msg}, nil)); err != nil {
		t.Fatalf("SaveToPM: %v", err)
	}
	copies := client.callsFor("copyMessage")
	if len(copies) != 1 || copies[0].Params["chat_id"] != int64(42) || copies[0].Params["message_id"] != int64(55) {
		t.Fatalf("SaveToPM: copyMessage calls = %v, want one to PM 42 of message 55", copies)
	}
	texts = client.sentTexts()
	if len(texts) != 1 || texts[0] != "https://t.me/c/123/55" {
		t.Errorf("SaveToPM sent %q, want post link", texts)
	}
	if n := len(client.callsFor("deleteMessage")); n != 1 {
		t.Errorf("SaveToPM: deleteMessage calls = %d, want 1", n)
	}

	// Album: all cached message ids of the group are copied at once.
	client.Calls = nil
	albumTarget := testMessage("альбом", testUser(777))
	albumTarget.MessageId = 60
	albumTarget.MediaGroupId = "test-album"
	ChatAlbumCache.mu.Lock()
	ChatAlbumCache.groups["test-album"] = []int64{60, 61}
	ChatAlbumCache.mu.Unlock()
	t.Cleanup(func() {
		ChatAlbumCache.mu.Lock()
		delete(ChatAlbumCache.groups, "test-album")
		ChatAlbumCache.mu.Unlock()
	})
	client.Results = map[string]json.RawMessage{"copyMessages": json.RawMessage(`[{"message_id":999}]`)}
	msg.ReplyToMessage = albumTarget
	if err := SaveToPM(bot, ext.NewContext(bot, &gotgbot.Update{UpdateId: 1, Message: msg}, nil)); err != nil {
		t.Fatalf("SaveToPM (album): %v", err)
	}
	copies = client.callsFor("copyMessages")
	if len(copies) != 1 {
		t.Fatalf("SaveToPM (album): copyMessages calls = %d, want 1", len(copies))
	}
	ids, ok := copies[0].Params["message_ids"].([]int64)
	if !ok || len(ids) != 2 {
		t.Errorf("SaveToPM (album): message_ids = %v, want two ids", copies[0].Params["message_ids"])
	}
}

// TestLoop covers the /loop argument validation (the ffmpeg conversion itself
// is not testable here).
func TestLoop(t *testing.T) {
	bot, client := setupTestEnv(t)

	// Usage message when not replying.
	if err := Loop(bot, testContext(bot, "/loop", testUser(42))); err != nil {
		t.Fatalf("Loop (usage): %v", err)
	}
	texts := client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "Пример использования") {
		t.Errorf("Loop (usage) sent %q, want usage message", texts)
	}

	// Reply without media.
	client.Calls = nil
	noMedia := testMessage("текст", testUser(777))
	if err := Loop(bot, testReplyContext(bot, "/loop", testUser(42), noMedia)); err != nil {
		t.Fatalf("Loop (no media): %v", err)
	}
	texts = client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "Какого-либо медиа нет") {
		t.Errorf("Loop (no media) sent %q, want missing-media message", texts)
	}

	// Photo media is not loopable.
	client.Calls = nil
	photo := testMessage("", testUser(777))
	photo.Photo = []gotgbot.PhotoSize{{FileId: "f1", Width: 10, Height: 10}}
	if err := Loop(bot, testReplyContext(bot, "/loop", testUser(42), photo)); err != nil {
		t.Fatalf("Loop (photo): %v", err)
	}
	texts = client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "Неподдерживаемая операция") {
		t.Errorf("Loop (photo) sent %q, want unsupported-operation message", texts)
	}
}
