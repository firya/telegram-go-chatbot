package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	cmc "github.com/miguelmota/go-coinmarketcap/pro/v1"
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

// stubProxy points httpClient() at a test server. Go sends the absolute URL of a
// plain http:// request to an HTTP proxy, so the handler below can answer for the
// service URLs the commands have hardcoded.
func stubProxy(t *testing.T, handler http.HandlerFunc) {
	t.Helper()

	old := HTTPClientProxy
	t.Cleanup(func() { HTTPClientProxy = old })

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	proxyURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("stubProxy: %v", err)
	}
	HTTPClientProxy = func(*http.Request) (*url.URL, error) { return proxyURL, nil }
}

// TestTldr covers the static hint about the TLDR button.
func TestTldr(t *testing.T) {
	bot, client := setupTestEnv(t)

	if err := TLDR(bot, testContext(bot, "/tldr", testUser(42))); err != nil {
		t.Fatalf("TLDR: %v", err)
	}
	texts := client.sentTexts()
	if len(texts) != 1 || texts[0] != "Юзайте кнопку TLDR напротив сообщения" {
		t.Errorf("TLDR sent %q, want the hint about the TLDR button", texts)
	}
}

// TestAdvice covers /advice against a stubbed advice API. The URL is hardcoded, so
// the request is intercepted with the proxy setting from config.json.
func TestAdvice(t *testing.T) {
	bot, client := setupTestEnv(t)

	stubProxy(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.RequestURI, "fucking-great-advice.ru/api/random") {
			http.Error(w, "unexpected target "+r.RequestURI, http.StatusNotFound)
			return
		}
		fmt.Fprint(w, `{"id":42,"text":"Проверяй тесты перед пушем.","sound":"meow"}`)
	})

	if err := Advice(bot, testContext(bot, "/advice", testUser(42))); err != nil {
		t.Fatalf("Advice: %v", err)
	}
	texts := client.sentTexts()
	if len(texts) != 1 || texts[0] != "Проверяй тесты перед пушем." {
		t.Errorf("Advice sent %q, want the advice text", texts)
	}

	// A non-JSON answer (a captcha page, an error page from the proxy) is an error.
	stubProxy(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "<html>captcha</html>")
	})
	client.Calls = nil
	if err := Advice(bot, testContext(bot, "/advice", testUser(42))); err == nil {
		t.Errorf("Advice: err = nil, want a decode error")
	}
	if texts := client.sentTexts(); len(texts) != 0 {
		t.Errorf("Advice sent %q on a broken answer, want nothing", texts)
	}
}

// TestAnekdot covers the parser of the anekdot.ru random page. The page embeds the
// text in a JSON.parse('[\"…\",\"…"]') blob; the old code indexed the split result
// straight away and panicked when the marker was missing.
func TestAnekdot(t *testing.T) {
	// A <br> between Cyrillic letters splits a sentence, so it becomes a space.
	page := `x JSON.parse('[\"Первый анекдот<br>продолжение фразы и \\\"кавычки\\\"\",\"next']);`
	got, err := parseAnekdot(page)
	if err != nil {
		t.Fatalf("parseAnekdot: %v", err)
	}
	if want := "Первый анекдот продолжение фразы и \"кавычки\""; got != want {
		t.Errorf("parseAnekdot = %q, want %q", got, want)
	}

	// Any other <br> stays a line break.
	got, err = parseAnekdot(`JSON.parse('[\"First line<br>second line\",\"next'])`)
	if err != nil {
		t.Fatalf("parseAnekdot (latin): %v", err)
	}
	if got != "First line\nsecond line" {
		t.Errorf("parseAnekdot (latin) = %q, want %q", got, "First line\nsecond line")
	}

	// A page without the marker (blocked request, changed format) is an error.
	if _, err := parseAnekdot("<html>503 Service Unavailable</html>"); err == nil {
		t.Errorf("parseAnekdot: err = nil on a page without the joke, want an error")
	}
}

// TestCur covers the parts of /cur that need no CoinMarketCap request: the
// not-configured reply and the argument checks.
func TestCur(t *testing.T) {
	bot, client := setupTestEnv(t)

	// No currency_key in config.json.
	if err := Cur(bot, testContext(bot, "/cur 1 USD RUB", testUser(42))); err != nil {
		t.Fatalf("Cur (no key): %v", err)
	}
	texts := client.sentTexts()
	if len(texts) != 1 || texts[0] != "Конвертация валют не настроена" {
		t.Errorf("Cur (no key) sent %q, want the not-configured reply", texts)
	}

	// With a key the argument count is checked first. /cur alone used to read
	// Args()[1] before that check and panicked.
	oldKey := Config.CurrencyKey
	t.Cleanup(func() { Config.CurrencyKey = oldKey })
	Config.CurrencyKey = "test-key"

	for _, args := range []string{"/cur", "/cur 1", "/cur 1 USD RUB extra"} {
		client.Calls = nil
		if err := Cur(bot, testContext(bot, args, testUser(42))); err != nil {
			t.Fatalf("Cur (usage %q): %v", args, err)
		}
		texts := client.sentTexts()
		if len(texts) != 1 || !strings.Contains(texts[0], "/cur 1 USD RUB") {
			t.Errorf("Cur (usage %q) sent %q, want the usage message", args, texts)
		}
	}

	client.Calls = nil
	if err := Cur(bot, testContext(bot, "/cur abc USD RUB", testUser(42))); err == nil {
		t.Errorf("Cur (bad amount): err = nil, want a ParseFloat error")
	}
}

// TestCurSymbolMaps covers the currency lookup. The fiat/crypto maps are seeded
// here, so no CoinMarketCap request is made.
func TestCurSymbolMaps(t *testing.T) {
	oldCrypto, oldFiat, oldCustom := CryptoMap, FiatMap, CustomMap
	t.Cleanup(func() { CryptoMap, FiatMap, CustomMap = oldCrypto, oldFiat, oldCustom })

	FiatMap = []*cmc.FiatMapListing{
		{ID: 9911, Name: "Russian Ruble", Symbol: "RUB"},
		{ID: 9913, Name: "US Dollar", Symbol: "USD"},
		{ID: 9916, Name: "Belarusian Ruble", Symbol: "BYN"},
		{ID: 9934, Name: "Turkish Lira", Symbol: "TRY"},
	}
	CryptoMap = []*cmc.MapListing{{ID: 1, Name: "Bitcoin", Symbol: "BTC"}}
	CustomMap = []CustomMapStruct{{symbol: "COC", name: "Cup Of Coffee", amount: 300.0}}

	for _, c := range []struct{ in, want string }{
		{"RUB", "9911"},
		{"usd", "9913"}, // the symbol is uppercased before the lookup
		{"byr", "9916"}, // the old Belarusian ruble resolves to BYN
		{"TL", "9934"},  // the old Turkish lira resolves to TRY
		{"coc", "9911"}, // custom symbols are worth rubles
		{"btc", "1"},    // crypto is searched after fiat
	} {
		got, err := GetSymbolId(c.in)
		if err != nil {
			t.Errorf("GetSymbolId(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("GetSymbolId(%q) = %q, want %q", c.in, got, c.want)
		}
	}

	if _, err := GetSymbolId("ZZZ"); err == nil || !strings.Contains(err.Error(), "распознать") {
		t.Errorf("GetSymbolId(ZZZ): err = %v, want the unknown currency message", err)
	}

	for _, c := range []struct{ id, want string }{
		{"9911", "Russian Ruble"},
		{"1", "Bitcoin"},
		{"777", ""},
	} {
		if got := GetIdName(c.id); got != c.want {
			t.Errorf("GetIdName(%q) = %q, want %q", c.id, got, c.want)
		}
	}

	// 9911 is the test currency of the API and is refused; the custom symbol
	// resolves to it, so /cur 1 COC RUB must not reach the network.
	bot, client := setupTestEnv(t)
	oldKey := Config.CurrencyKey
	t.Cleanup(func() { Config.CurrencyKey = oldKey })
	Config.CurrencyKey = "test-key"

	if err := Cur(bot, testContext(bot, "/cur 1 COC RUB", testUser(42))); err == nil ||
		!strings.Contains(err.Error(), "тестовую валюту") {
		t.Errorf("Cur (test currency): err = %v, want the test currency refusal", err)
	}
	if texts := client.sentTexts(); len(texts) != 0 {
		t.Errorf("Cur (test currency) sent %q, want nothing", texts)
	}
}
