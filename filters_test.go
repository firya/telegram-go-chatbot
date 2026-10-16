package main

import (
	"encoding/json"
	"fmt"
	"io"
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

// TestGlobalMiddlewareAntispam covers the anti-spam part of the global
// middleware: the threshold from config.json (the report fires once the window
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

	// Crossing the threshold reports to the sysadmin in PM.
	if err := m.HandleUpdate(bot, testContext(bot, "привет", testUser(42))); err != nil {
		t.Fatalf("HandleUpdate (threshold): %v", err)
	}
	calls := client.callsFor("sendMessage")
	if len(calls) != 1 {
		t.Fatalf("middleware sendMessage calls = %d, want 1", len(calls))
	}
	if calls[0].Params["chat_id"] != int64(Config.SysAdmin) {
		t.Errorf("spam report chat_id = %v, want sysadmin %d", calls[0].Params["chat_id"], Config.SysAdmin)
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

// TestErrorReporting covers the sysadmin notification: the bot token must not
// leak into the message.
func TestErrorReporting(t *testing.T) {
	_, client := setupTestEnv(t)
	Config.Token = "123456:TESTTOKEN"

	ErrorReporting(fmt.Errorf("request failed with %v", Config.Token))

	calls := client.callsFor("sendMessage")
	if len(calls) != 1 {
		t.Fatalf("ErrorReporting sendMessage calls = %d, want 1", len(calls))
	}
	if calls[0].Params["chat_id"] != int64(Config.SysAdmin) {
		t.Errorf("ErrorReporting chat_id = %v, want sysadmin %d", calls[0].Params["chat_id"], Config.SysAdmin)
	}
	text, _ := calls[0].Params["text"].(string)
	if strings.Contains(text, "TESTTOKEN") {
		t.Errorf("ErrorReporting leaked the token: %q", text)
	}
	if !strings.Contains(text, "request failed with TOKEN") {
		t.Errorf("ErrorReporting sent %q, want the token replaced with TOKEN", text)
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
// config.json, and the startup report must reach the sysadmin.
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
	// API URL from config.json, with the token in the path.
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

	// The startup report is sent to the sysadmin from config.json.
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
	if !strings.Contains(report.body, "name=\"chat_id\"\r\n\r\n7\r\n") {
		t.Errorf("BotInit startup report = %q, want it addressed to the sysadmin 7", report.body)
	}
	if !strings.Contains(report.body, "Connection type: polling") {
		t.Errorf("BotInit startup report = %q, want the connection type in it", report.body)
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
