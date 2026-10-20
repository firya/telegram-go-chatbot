package main

import (
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// assertUntilDate checks that a restriction until_date param is close to the
// expected unix time (handlers compute it with time.Now()).
func assertUntilDate(t *testing.T, params map[string]any, want time.Time) {
	t.Helper()
	until, ok := params["until_date"].(int64)
	if !ok {
		t.Errorf("until_date param missing or not an int64: %v", params["until_date"])
		return
	}
	diff := time.Until(time.Unix(until, 0)) - time.Until(want)
	if diff < -5*time.Second || diff > 5*time.Second {
		t.Errorf("until_date = %v, want ~%v", time.Unix(until, 0).Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

func TestBan(t *testing.T) {
	bot, client := setupTestEnv(t)

	// Non-admin -> Denied.
	if err := Ban(bot, testContext(bot, "/ban 777", testUser(42))); err != nil {
		t.Fatalf("Ban (non-admin): %v", err)
	}
	if texts := client.sentTexts(); len(texts) != 1 || texts[0] != "нет" {
		t.Errorf("Ban (non-admin) sent %q, want Nope text", texts)
	}

	// Usage message.
	client.Calls = nil
	if err := Ban(bot, testContext(bot, "/ban", testUser(100))); err != nil {
		t.Fatalf("Ban (usage): %v", err)
	}
	texts := client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "Пример использования") {
		t.Errorf("Ban (usage) sent %q, want usage message", texts)
	}

	// Ban via reply: default 24h restriction.
	client.Calls = nil
	replyTo := testMessage("сообщение", &targetUser)
	if err := Ban(bot, testReplyContext(bot, "/ban", testUser(100), replyTo)); err != nil {
		t.Fatalf("Ban (reply): %v", err)
	}
	calls := client.callsFor("banChatMember")
	if len(calls) != 1 {
		t.Fatalf("Ban (reply): banChatMember calls = %d, want 1", len(calls))
	}
	assertUntilDate(t, calls[0].Params, time.Now().Add(24*time.Hour))
	texts = client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "забанен до") {
		t.Errorf("Ban (reply) sent %q, want ban message with deadline", texts)
	}

	// Ban by argument: the target ID must not be mistaken for a duration.
	seedUser(777, "target", "Цель")
	client.Calls = nil
	if err := Ban(bot, testContext(bot, "/ban 777", testUser(100))); err != nil {
		t.Fatalf("Ban (arg): %v", err)
	}
	calls = client.callsFor("banChatMember")
	if len(calls) != 1 {
		t.Fatalf("Ban (arg): banChatMember calls = %d, want 1", len(calls))
	}
	assertUntilDate(t, calls[0].Params, time.Now().Add(24*time.Hour))

	// Ban by argument with an explicit duration.
	client.Calls = nil
	if err := Ban(bot, testContext(bot, "/ban 777 3600", testUser(100))); err != nil {
		t.Fatalf("Ban (arg+time): %v", err)
	}
	calls = client.callsFor("banChatMember")
	if len(calls) != 1 {
		t.Fatalf("Ban (arg+time): banChatMember calls = %d, want 1", len(calls))
	}
	assertUntilDate(t, calls[0].Params, time.Now().Add(3600*time.Second))
}

func TestKick(t *testing.T) {
	bot, client := setupTestEnv(t)

	// Non-admin -> Denied.
	if err := Kick(bot, testContext(bot, "/kick 777", testUser(42))); err != nil {
		t.Fatalf("Kick (non-admin): %v", err)
	}
	if texts := client.sentTexts(); len(texts) != 1 || texts[0] != "нет" {
		t.Errorf("Kick (non-admin) sent %q, want Nope text", texts)
	}

	// Usage message.
	client.Calls = nil
	if err := Kick(bot, testContext(bot, "/kick", testUser(100))); err != nil {
		t.Fatalf("Kick (usage): %v", err)
	}
	texts := client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "Пример использования") {
		t.Errorf("Kick (usage) sent %q, want usage message", texts)
	}

	// Kick via reply: ban + unban (kick while allowing rejoin).
	client.Calls = nil
	replyTo := testMessage("сообщение", &targetUser)
	if err := Kick(bot, testReplyContext(bot, "/kick", testUser(100), replyTo)); err != nil {
		t.Fatalf("Kick: %v", err)
	}
	if n := len(client.callsFor("banChatMember")); n != 1 {
		t.Errorf("Kick: banChatMember calls = %d, want 1", n)
	}
	if n := len(client.callsFor("unbanChatMember")); n != 1 {
		t.Errorf("Kick: unbanChatMember calls = %d, want 1", n)
	}
	texts = client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "исключен") {
		t.Errorf("Kick sent %q, want kick message", texts)
	}
}

func TestMute(t *testing.T) {
	bot, client := setupTestEnv(t)

	// Non-admin -> Denied.
	if err := Mute(bot, testContext(bot, "/mute 777", testUser(42))); err != nil {
		t.Fatalf("Mute (non-admin): %v", err)
	}
	if texts := client.sentTexts(); len(texts) != 1 || texts[0] != "нет" {
		t.Errorf("Mute (non-admin) sent %q, want Nope text", texts)
	}

	// Mute via reply: default 24h restriction.
	client.Calls = nil
	replyTo := testMessage("сообщение", &targetUser)
	if err := Mute(bot, testReplyContext(bot, "/mute", testUser(100), replyTo)); err != nil {
		t.Fatalf("Mute (reply): %v", err)
	}
	calls := client.callsFor("restrictChatMember")
	if len(calls) != 1 {
		t.Fatalf("Mute (reply): restrictChatMember calls = %d, want 1", len(calls))
	}
	assertUntilDate(t, calls[0].Params, time.Now().Add(24*time.Hour))
	texts := client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "больше не может отправлять сообщения до") {
		t.Errorf("Mute (reply) sent %q, want mute message with deadline", texts)
	}
	// The default day of mute is counted into the mute-time stats (stat type 7).
	if got := muteSeconds(t, 777); got != 86400 {
		t.Errorf("Mute (reply): mute seconds = %d, want 86400", got)
	}

	// Mute by argument: the target ID must not be mistaken for a duration.
	seedUser(777, "target", "Цель")
	client.Calls = nil
	if err := Mute(bot, testContext(bot, "/mute 777", testUser(100))); err != nil {
		t.Fatalf("Mute (arg): %v", err)
	}
	calls = client.callsFor("restrictChatMember")
	if len(calls) != 1 {
		t.Fatalf("Mute (arg): restrictChatMember calls = %d, want 1", len(calls))
	}
	assertUntilDate(t, calls[0].Params, time.Now().Add(24*time.Hour))
	if got := muteSeconds(t, 777); got != 2*86400 {
		t.Errorf("Mute (arg): mute seconds = %d, want 172800", got)
	}

	// Mute by argument with an explicit duration.
	client.Calls = nil
	if err := Mute(bot, testContext(bot, "/mute 777 600", testUser(100))); err != nil {
		t.Fatalf("Mute (arg+time): %v", err)
	}
	calls = client.callsFor("restrictChatMember")
	if len(calls) != 1 {
		t.Fatalf("Mute (arg+time): restrictChatMember calls = %d, want 1", len(calls))
	}
	assertUntilDate(t, calls[0].Params, time.Now().Add(600*time.Second))
	if got := muteSeconds(t, 777); got != 2*86400+600 {
		t.Errorf("Mute (arg+time): mute seconds = %d, want 173400", got)
	}
}

func TestMuteTop(t *testing.T) {
	bot, client := setupTestEnv(t)

	// Empty top.
	if err := MuteTop(bot, testContext(bot, "/mutetop", testUser(42))); err != nil {
		t.Fatalf("MuteTop (empty): %v", err)
	}
	texts := client.sentTexts()
	if len(texts) != 1 || texts[0] != "Топ-10 времени, проведённого в муте:\n\n" {
		t.Errorf("MuteTop (empty) sent %q, want empty top", texts)
	}

	// Ranked users; known users are shown by name, unknown by id. The seconds
	// of several days are summed, and the durations are spelled out declined.
	client.Calls = nil
	seedUser(42, "test42", "Тест")
	DB.Create(&Stats{ContextID: 42, StatType: 7, DayTimestamp: 1, Count: 86400})
	DB.Create(&Stats{ContextID: 42, StatType: 7, DayTimestamp: 2, Count: 7200})
	DB.Create(&Stats{ContextID: 777, StatType: 7, DayTimestamp: 1, Count: 90})
	if err := MuteTop(bot, testContext(bot, "/mutetop", testUser(42))); err != nil {
		t.Fatalf("MuteTop: %v", err)
	}
	texts = client.sentTexts()
	if len(texts) != 1 ||
		!strings.Contains(texts[0], "1. Тест - 1 день 2 часа") ||
		!strings.Contains(texts[0], "2. 777 - 1 минута 30 секунд") {
		t.Errorf("MuteTop sent %q, want ranked top with formatted durations", texts)
	}

	// Other stat types do not leak into the top.
	client.Calls = nil
	DB.Create(&Stats{ContextID: 500, StatType: 3, DayTimestamp: 1, Count: 100000})
	if err := MuteTop(bot, testContext(bot, "/mutetop", testUser(42))); err != nil {
		t.Fatalf("MuteTop (other types): %v", err)
	}
	if texts := client.sentTexts(); len(texts) != 1 || strings.Contains(texts[0], "500") {
		t.Errorf("MuteTop (other types) sent %q, want the message counter excluded", texts)
	}
}

func TestUnban(t *testing.T) {
	bot, client := setupTestEnv(t)

	// Non-admin -> Denied.
	if err := Unban(bot, testContext(bot, "/unban 777", testUser(42))); err != nil {
		t.Fatalf("Unban (non-admin): %v", err)
	}
	if texts := client.sentTexts(); len(texts) != 1 || texts[0] != "нет" {
		t.Errorf("Unban (non-admin) sent %q, want Nope text", texts)
	}

	// Usage message.
	client.Calls = nil
	if err := Unban(bot, testContext(bot, "/unban", testUser(100))); err != nil {
		t.Fatalf("Unban (usage): %v", err)
	}
	texts := client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "Пример использования") {
		t.Errorf("Unban (usage) sent %q, want usage message", texts)
	}

	// Unban via reply.
	client.Calls = nil
	replyTo := testMessage("сообщение", &targetUser)
	if err := Unban(bot, testReplyContext(bot, "/unban", testUser(100), replyTo)); err != nil {
		t.Fatalf("Unban: %v", err)
	}
	if n := len(client.callsFor("unbanChatMember")); n != 1 {
		t.Errorf("Unban: unbanChatMember calls = %d, want 1", n)
	}
	texts = client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "разбанен") {
		t.Errorf("Unban sent %q, want unban message", texts)
	}

	// Unbanning the bot itself -> animation easter egg instead.
	client.Calls = nil
	botMsg := testMessage("ботовское", &gotgbot.User{Id: Bot.Id, IsBot: true, FirstName: "Bot"})
	if err := Unban(bot, testReplyContext(bot, "/unban", testUser(100), botMsg)); err != nil {
		t.Fatalf("Unban (bot): %v", err)
	}
	if n := len(client.callsFor("sendAnimation")); n != 1 {
		t.Errorf("Unban (bot): sendAnimation calls = %d, want 1", n)
	}
	if n := len(client.callsFor("unbanChatMember")); n != 0 {
		t.Errorf("Unban (bot): unbanChatMember calls = %d, want 0", n)
	}
}

func TestUnmute(t *testing.T) {
	bot, client := setupTestEnv(t)

	// Non-admin -> Denied.
	if err := Unmute(bot, testContext(bot, "/unmute 777", testUser(42))); err != nil {
		t.Fatalf("Unmute (non-admin): %v", err)
	}
	if texts := client.sentTexts(); len(texts) != 1 || texts[0] != "нет" {
		t.Errorf("Unmute (non-admin) sent %q, want Nope text", texts)
	}

	// Usage message.
	client.Calls = nil
	if err := Unmute(bot, testContext(bot, "/unmute", testUser(100))); err != nil {
		t.Fatalf("Unmute (usage): %v", err)
	}
	texts := client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "Пример использования") {
		t.Errorf("Unmute (usage) sent %q, want usage message", texts)
	}

	// Unmute via reply: full permissions restored.
	client.Calls = nil
	replyTo := testMessage("сообщение", &targetUser)
	if err := Unmute(bot, testReplyContext(bot, "/unmute", testUser(100), replyTo)); err != nil {
		t.Fatalf("Unmute: %v", err)
	}
	calls := client.callsFor("restrictChatMember")
	if len(calls) != 1 {
		t.Fatalf("Unmute: restrictChatMember calls = %d, want 1", len(calls))
	}
	assertUntilDate(t, calls[0].Params, time.Now().Add(60*time.Second))
	texts = client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "снова может отправлять сообщения") {
		t.Errorf("Unmute sent %q, want unmute message", texts)
	}
}

func TestRevive(t *testing.T) {
	bot, client := setupTestEnv(t)

	// Non-admin -> Denied.
	if err := Revive(bot, testContext(bot, "/revive 777", testUser(42))); err != nil {
		t.Fatalf("Revive (non-admin): %v", err)
	}
	if texts := client.sentTexts(); len(texts) != 1 || texts[0] != "нет" {
		t.Errorf("Revive (non-admin) sent %q, want Nope text", texts)
	}

	// Revive via reply.
	client.Calls = nil
	replyTo := testMessage("сообщение", &targetUser)
	if err := Revive(bot, testReplyContext(bot, "/revive", testUser(100), replyTo)); err != nil {
		t.Fatalf("Revive: %v", err)
	}
	calls := client.callsFor("restrictChatMember")
	if len(calls) != 1 {
		t.Fatalf("Revive: restrictChatMember calls = %d, want 1", len(calls))
	}
	texts := client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "возродился в чате") {
		t.Errorf("Revive sent %q, want revive message", texts)
	}
}
