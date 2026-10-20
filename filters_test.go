package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path"
	"slices"
	"strings"
	"sync"
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

// TestLoop covers the /loop argument validation. The conversion itself, with
// real ffmpeg, is TestLoopAnimation in media_test.go.
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

// TestGlobalMiddlewareAccessors covers the two questions the dispatcher asks a
// middleware before running it: the name it is identified by, and whether it
// wants the update. The global middleware takes everything, which is what
// makes it see all updates.
func TestGlobalMiddlewareAccessors(t *testing.T) {
	bot, _ := setupTestEnv(t)

	m := NewGlobalMiddleware()
	if name := m.Name(); name != "global_middleware" {
		t.Errorf("GlobalMiddleware.Name() = %q, want global_middleware", name)
	}
	if !m.CheckUpdate(bot, testContext(bot, "/ping", testUser(42))) {
		t.Error("CheckUpdate (message) = false, want every update accepted")
	}
	if !m.CheckUpdate(bot, ext.NewContext(bot, &gotgbot.Update{UpdateId: 1}, nil)) {
		t.Error("CheckUpdate (empty update) = false, want every update accepted")
	}
}

// TestGlobalMiddlewareAntispam covers the anti-spam part of the global
// middleware: the threshold from the config (the report fires once the window
// holds antispam_count earlier messages, so with 3/30s in the test config the
// 4th message reports), the 10 second throttle, the window expiry and updates
// without a sender.
func TestGlobalMiddlewareAntispam(t *testing.T) {
	bot, client := setupTestEnv(t)

	m := NewGlobalMiddleware()

	// The counter is the number of earlier messages in the window, so the
	// threshold is crossed by the message after the last one counted.
	for i := 0; i < Config.AntiSpamCount; i++ {
		if err := m.HandleUpdate(bot, testContext(bot, "привет", testUser(42))); err != nil {
			t.Fatalf("HandleUpdate (%d): %v", i, err)
		}
	}
	if texts := client.sentTexts(); len(texts) != 0 {
		t.Errorf("middleware sent %q below the threshold, want nothing", texts)
	}

	// Crossing the threshold reports to the sysadmin as an ephemeral message in
	// the main chat.
	if err := m.HandleUpdate(bot, testContext(bot, "привет", testUser(42))); err != nil {
		t.Fatalf("HandleUpdate (threshold): %v", err)
	}
	calls := client.callsFor("sendMessage")
	if len(calls) != 1 {
		t.Fatalf("middleware sendMessage calls = %d, want 1", len(calls))
	}
	if calls[0].Params["chat_id"] != int64(Config.Chat) {
		t.Errorf("spam report chat_id = %v, want the main chat %d", calls[0].Params["chat_id"], Config.Chat)
	}
	eph, ok := calls[0].Params["ephemeral_message_parameters"].(*gotgbot.EphemeralMessageParameters)
	if !ok || eph.ReceiverUserId != int64(Config.SysAdmin) {
		t.Errorf("spam report ephemeral_message_parameters = %v, want the sysadmin %d", calls[0].Params["ephemeral_message_parameters"], Config.SysAdmin)
	}
	text, _ := calls[0].Params["text"].(string)
	if !strings.Contains(text, "Возможно спам") || !strings.Contains(text, "привет") {
		t.Errorf("spam report = %q, want the spam note with the message text", text)
	}

	// The report is throttled: the next burst within 10 seconds is silent.
	client.Calls = nil
	for i := 0; i < Config.AntiSpamCount; i++ {
		if err := m.HandleUpdate(bot, testContext(bot, "привет", testUser(42))); err != nil {
			t.Fatalf("HandleUpdate (throttle): %v", err)
		}
	}
	if calls := client.callsFor("sendMessage"); len(calls) != 0 {
		t.Errorf("throttle: sendMessage calls = %d, want 0", len(calls))
	}

	// Timestamps older than the window are dropped, so an old burst does not
	// count towards the new one.
	old := NewGlobalMiddleware()
	old.timestamps[42] = []time.Time{
		time.Now().Add(-100 * time.Second),
		time.Now().Add(-90 * time.Second),
		time.Now().Add(-80 * time.Second),
	}
	client.Calls = nil
	if err := old.HandleUpdate(bot, testContext(bot, "привет", testUser(42))); err != nil {
		t.Fatalf("HandleUpdate (expired window): %v", err)
	}
	if calls := client.callsFor("sendMessage"); len(calls) != 0 {
		t.Errorf("expired window: sendMessage calls = %d, want 0", len(calls))
	}
	if n := len(old.timestamps[42]); n != 1 {
		t.Errorf("expired window: stored timestamps = %d, want only the current one", n)
	}

	// An update without a sender is logged only.
	noSender := NewGlobalMiddleware()
	update := &gotgbot.Update{
		UpdateId:    5,
		ChannelPost: &gotgbot.Message{Text: "пост", Chat: gotgbot.Chat{Id: -100, Type: "channel", Title: "канал"}},
	}
	if err := noSender.HandleUpdate(bot, ext.NewContext(bot, update, nil)); err != nil {
		t.Fatalf("HandleUpdate (channel post): %v", err)
	}
	if calls := client.callsFor("sendMessage"); len(calls) != 0 {
		t.Errorf("channel post: sendMessage calls = %d, want 0", len(calls))
	}
}

// TestErrorReporting covers the sysadmin notification: the text goes to the
// main chat as an ephemeral message for the sysadmin, and the bot token must
// not leak into it.
func TestErrorReporting(t *testing.T) {
	_, client := setupTestEnv(t)
	Config.Token = "123456:TESTTOKEN"

	ErrorReporting(fmt.Errorf("request failed with %v", Config.Token))

	calls := client.callsFor("sendMessage")
	if len(calls) != 1 {
		t.Fatalf("ErrorReporting sendMessage calls = %d, want 1", len(calls))
	}
	if calls[0].Params["chat_id"] != int64(Config.Chat) {
		t.Errorf("ErrorReporting chat_id = %v, want the main chat %d", calls[0].Params["chat_id"], Config.Chat)
	}
	eph, ok := calls[0].Params["ephemeral_message_parameters"].(*gotgbot.EphemeralMessageParameters)
	if !ok || eph.ReceiverUserId != int64(Config.SysAdmin) {
		t.Errorf("ErrorReporting ephemeral_message_parameters = %v, want the sysadmin %d", calls[0].Params["ephemeral_message_parameters"], Config.SysAdmin)
	}
	text, _ := calls[0].Params["text"].(string)
	if strings.Contains(text, "TESTTOKEN") {
		t.Errorf("ErrorReporting leaked the token: %q", text)
	}
	if !strings.Contains(text, "request failed with TOKEN") {
		t.Errorf("ErrorReporting sent %q, want the token replaced with TOKEN", text)
	}
}

// TestUpdateContext covers the error-report footer: the command text with its
// arguments, the sender and the chat, HTML escaped so the report sends.
func TestUpdateContext(t *testing.T) {
	bot, _ := setupTestEnv(t)

	// Message with arguments: text, sender mention and chat id, markup
	// escaped.
	got := updateContext(testContext(bot, "/mp3 https://youtu.be/x <b>", testUser(42)))
	if !strings.Contains(got, "/mp3 https://youtu.be/x &lt;b&gt;") ||
		!strings.Contains(got, "Юзер: <a href=\"tg://user?id=42\">Тест") ||
		!strings.Contains(got, "Чат: -100") {
		t.Errorf("updateContext (message) = %q, want command, user and chat", got)
	}

	// Message without text falls back to its id.
	if got := updateContext(testContext(bot, "", testUser(42))); !strings.Contains(got, "сообщение #100 без текста") {
		t.Errorf("updateContext (no text) = %q, want the message id fallback", got)
	}

	// Callbacks and inline queries show their payload.
	msg := testMessage("duel", testUser(777))
	cb := ext.NewContext(bot, &gotgbot.Update{UpdateId: 1, CallbackQuery: &gotgbot.CallbackQuery{Id: "1", From: *testUser(42), Message: *msg, Data: "duel_accept"}}, nil)
	if got := updateContext(cb); !strings.Contains(got, "callback: duel_accept") {
		t.Errorf("updateContext (callback) = %q, want the callback data", got)
	}
	iq := testInlineContext(bot, testUser(42), "привет")
	if got := updateContext(iq); !strings.Contains(got, "inline: привет") {
		t.Errorf("updateContext (inline) = %q, want the inline query", got)
	}

	// Without a context there is nothing to describe.
	if got := updateContext(nil); !strings.Contains(got, "неизвестно") {
		t.Errorf("updateContext (nil) = %q, want the unknown fallback", got)
	}
}

// TestYtdlpGetVer reports the version line of the installed yt-dlp, and
// "Unknown" when the binary is not on the machine — the answer /debug shows
// then. The version itself is not pinned, only its shape.
func TestYtdlpGetVer(t *testing.T) {
	_, lookErr := exec.LookPath("yt-dlp")
	got := ytdlpGetVer()

	if lookErr != nil {
		if got != "Unknown" {
			t.Errorf("ytdlpGetVer = %q, want %q without yt-dlp", got, "Unknown")
		}
		t.Skip("yt-dlp is not installed, the version line is not checked")
	}

	if got == "Unknown" {
		t.Errorf("ytdlpGetVer = %q, want the version line of the installed yt-dlp", got)
	}
	if strings.ContainsAny(got, "\n") {
		t.Errorf("ytdlpGetVer = %q, want one line without the trailing newline", got)
	}
}

// botAPICall is one request a test server has seen.
type botAPICall struct {
	method string
	path   string
	body   string
}

// botAPICalls collects the Bot API requests a test server has seen.
type botAPICalls struct {
	mu    sync.Mutex
	calls []botAPICall
}

func (c *botAPICalls) add(method, urlPath, body string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, botAPICall{method: method, path: urlPath, body: body})
}

func (c *botAPICalls) all() []botAPICall {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]botAPICall{}, c.calls...)
}

// hasCall reports whether the list contains a method.
func hasCall(calls []botAPICall, method string) bool {
	return slices.ContainsFunc(calls, func(c botAPICall) bool { return c.method == method })
}

// waitForCallsContaining waits until the server has seen every method in want.
func waitForCallsContaining(t *testing.T, calls *botAPICalls, want ...string) []botAPICall {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for {
		got := calls.all()
		found := true
		for _, w := range want {
			if !hasCall(got, w) {
				found = false
				break
			}
		}
		if found {
			return got
		}
		if time.Now().After(deadline) {
			return got
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestBotInit starts the bot against a local stand-in for the Bot API: the
// token check, the webhook deletion and the polling must all go to the URL from
// ZAVTRA_BOT_API_URL, and the startup report must reach the sysadmin.
func TestBotInit(t *testing.T) {
	setupTestEnv(t)

	oldBot, oldDispatcher, oldUpdater, oldConfig := Bot, BotDispatcher, BotUpdater, Config

	calls := &botAPICalls{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := path.Base(r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		calls.add(method, r.URL.Path, string(body))

		w.Header().Set("Content-Type", "application/json")
		switch method {
		case "getMe":
			fmt.Fprint(w, `{"ok":true,"result":{"id":42,"is_bot":true,"first_name":"test","username":"testbot"}}`)
		case "getUpdates":
			// Long polling: answer slowly so the polling loop does not spin.
			time.Sleep(200 * time.Millisecond)
			fmt.Fprint(w, `{"ok":true,"result":[]}`)
		case "sendMessage":
			fmt.Fprint(w, `{"ok":true,"result":{"message_id":1,"date":1700000000,`+
				`"chat":{"id":7,"type":"private","first_name":"sysadmin"},`+
				`"from":{"id":42,"is_bot":true,"first_name":"test","username":"testbot"}}}`)
		default:
			fmt.Fprint(w, `{"ok":true,"result":true}`)
		}
	}))

	// Cleanups run in reverse order: stop the polling first, then hand the
	// globals back, then close the server. The other way round the polling
	// goroutine reports the closed server to ErrorReporting after the test is
	// done.
	t.Cleanup(server.Close)
	t.Cleanup(func() {
		Bot, BotDispatcher, BotUpdater, Config = oldBot, oldDispatcher, oldUpdater, oldConfig
	})
	t.Cleanup(func() {
		if BotUpdater != nil {
			BotUpdater.Stop()
		}
		time.Sleep(300 * time.Millisecond)
	})

	Config.BotApiUrl = server.URL
	Config.Token = "123:abc"
	Config.AllowedUpdates = []string{"message"}
	Config.SysAdmin = 7
	Config.EndpointPublicURL, Config.Listen = "", ""
	Bot, BotDispatcher, BotUpdater = nil, nil, nil

	if err := BotInit(); err != nil {
		t.Fatalf("BotInit: %v", err)
	}
	if Bot == nil {
		t.Fatalf("BotInit: Bot is nil")
	}
	if BotDispatcher == nil {
		t.Errorf("BotInit: BotDispatcher is nil")
	}
	if BotUpdater == nil {
		t.Errorf("BotInit: BotUpdater is nil")
	}
	if Bot.Username != "testbot" {
		t.Errorf("Bot.Username = %q, want the name from getMe", Bot.Username)
	}

	seen := waitForCallsContaining(t, calls, "getMe", "deleteWebhook", "getUpdates", "sendMessage")

	// The token check, the polling start and the sysadmin report all go to the
	// API URL of the config, with the token in the path.
	for _, entry := range seen {
		if !strings.HasPrefix(entry.path, "/bot123:abc/") {
			t.Errorf("%v request path = %q, want it under /bot123:abc/", entry.method, entry.path)
		}
	}
	for _, want := range []string{"getMe", "deleteWebhook", "getUpdates", "sendMessage"} {
		if !hasCall(seen, want) {
			t.Errorf("BotInit: the API never saw %v", want)
		}
	}

	// The startup report is an ephemeral message for the sysadmin in the main chat.
	var report *botAPICall
	for i, entry := range seen {
		if entry.method == "sendMessage" {
			report = &seen[i]
		}
	}
	if report == nil {
		t.Fatalf("BotInit: no sendMessage to the sysadmin")
	}
	// The API sends the parameters as multipart form data.
	if !strings.Contains(report.body, "name=\"chat_id\"\r\n\r\n-100\r\n") {
		t.Errorf("BotInit startup report = %q, want it addressed to the main chat %d", report.body, Config.Chat)
	}
	if !strings.Contains(report.body, "name=\"ephemeral_message_parameters\"\r\n\r\n{\"receiver_user_id\":7}\r\n") {
		t.Errorf("BotInit startup report = %q, want it addressed to the sysadmin 7", report.body)
	}
	if !strings.Contains(report.body, "Connection: polling") {
		t.Errorf("BotInit startup report = %q, want the connection type in it", report.body)
	}
}

// TestBotInitWebhook starts the bot in webhook mode: the update endpoint is
// listening before the webhook is set, the API is told the public URL with the
// bot username as the path, and updates that do not carry the secret token the
// server was started with are dropped.
func TestBotInitWebhook(t *testing.T) {
	setupTestEnv(t)

	oldBot, oldDispatcher, oldUpdater, oldConfig := Bot, BotDispatcher, BotUpdater, Config

	// StartWebhook listens synchronously, so the port picked here is the
	// address the update endpoint answers on.
	free, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	listen := fmt.Sprintf("127.0.0.1:%d", free.Addr().(*net.TCPAddr).Port)
	free.Close()

	calls := &botAPICalls{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := path.Base(r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		calls.add(method, r.URL.Path, string(body))

		w.Header().Set("Content-Type", "application/json")
		switch method {
		case "getMe":
			fmt.Fprint(w, `{"ok":true,"result":{"id":42,"is_bot":true,"first_name":"test","username":"testbot"}}`)
		case "sendMessage":
			fmt.Fprint(w, `{"ok":true,"result":{"message_id":1,"date":1700000000,`+
				`"chat":{"id":7,"type":"private","first_name":"sysadmin"},`+
				`"from":{"id":42,"is_bot":true,"first_name":"test","username":"testbot"}}}`)
		default:
			fmt.Fprint(w, `{"ok":true,"result":true}`)
		}
	}))

	// The same cleanup order as TestBotInit: stop the updater (which closes the
	// webhook server), hand the globals back, close the API.
	t.Cleanup(server.Close)
	t.Cleanup(func() {
		Bot, BotDispatcher, BotUpdater, Config = oldBot, oldDispatcher, oldUpdater, oldConfig
	})
	t.Cleanup(func() {
		if BotUpdater != nil {
			BotUpdater.Stop()
		}
		time.Sleep(300 * time.Millisecond)
	})

	Config.BotApiUrl = server.URL
	Config.Token = "123:abc"
	Config.AllowedUpdates = []string{"message"}
	Config.SysAdmin = 7
	Config.MaxConnections = 40
	Config.EndpointPublicURL = "https://example.invalid/"
	Config.Listen = listen
	Bot, BotDispatcher, BotUpdater = nil, nil, nil

	if err := BotInit(); err != nil {
		t.Fatalf("BotInit (webhook): %v", err)
	}

	seen := waitForCallsContaining(t, calls, "getMe", "setWebhook", "sendMessage")
	if hasCall(seen, "getUpdates") {
		t.Errorf("BotInit (webhook): the API saw getUpdates, want no polling")
	}

	var webhook, report *botAPICall
	for i, entry := range seen {
		if entry.method == "setWebhook" {
			webhook = &seen[i]
		}
		if entry.method == "sendMessage" {
			report = &seen[i]
		}
	}
	if webhook == nil {
		t.Fatalf("BotInit (webhook): the API never saw setWebhook")
	}
	// The endpoint is the public URL of the config with the bot username as
	// the path, set with the secret token and the connection limit.
	if !strings.Contains(webhook.body, "https://example.invalid/testbot") {
		t.Errorf("BotInit (webhook) setWebhook = %q, want the endpoint under the public URL", webhook.body)
	}
	if !strings.Contains(webhook.body, "name=\"secret_token\"") {
		t.Errorf("BotInit (webhook) setWebhook = %q, want the secret token in it", webhook.body)
	}
	if !strings.Contains(webhook.body, "name=\"max_connections\"\r\n\r\n40\r\n") {
		t.Errorf("BotInit (webhook) setWebhook = %q, want max_connections 40", webhook.body)
	}
	if report == nil {
		t.Fatalf("BotInit (webhook): no sendMessage to the sysadmin")
	}
	if !strings.Contains(report.body, "Connection: webhook") {
		t.Errorf("BotInit (webhook) startup report = %q, want the webhook connection type", report.body)
	}

	// The endpoint is live and enforces the secret: an update without the
	// secret token header is dropped, a path of another bot is not served here.
	post := func(p string) int {
		t.Helper()
		resp, err := http.Post("http://"+listen+p, "application/json",
			strings.NewReader(`{"update_id":1,"message":{"message_id":1,"date":1700000000,"chat":{"id":7}}}`))
		if err != nil {
			t.Fatalf("webhook request to %v: %v", p, err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}
	if code := post("/testbot"); code != http.StatusUnauthorized {
		t.Errorf("webhook POST /testbot without the secret = %d, want 401", code)
	}
	if code := post("/otherbot"); code != http.StatusNotFound {
		t.Errorf("webhook POST /otherbot = %d, want 404", code)
	}
}

// TestBotInitTokenFailure covers the start with a Bot API that refuses the
// token: the error goes back to main, and the globals stay untouched.
func TestBotInitTokenFailure(t *testing.T) {
	setupTestEnv(t)

	oldBot, oldDispatcher, oldUpdater, oldConfig := Bot, BotDispatcher, BotUpdater, Config
	t.Cleanup(func() {
		Bot, BotDispatcher, BotUpdater, Config = oldBot, oldDispatcher, oldUpdater, oldConfig
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"ok":false,"description":"Bad Request: token invalid"}`)
	}))
	t.Cleanup(server.Close)

	Config.BotApiUrl = server.URL
	Config.Token = "123:abc"
	Bot, BotDispatcher, BotUpdater = nil, nil, nil

	if err := BotInit(); err == nil {
		t.Errorf("BotInit (bad token): err = nil, want the token check error")
	}
	if Bot != nil {
		t.Errorf("BotInit (bad token): Bot = %v, want the globals untouched", Bot)
	}
}
