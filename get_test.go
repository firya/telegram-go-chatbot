package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// paramFileID extracts the file id from a gotgbot file param (InputFileByID
// returns an unexported FileReader which marshals to the file id string).
func paramFileID(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Errorf("marshal file param: %v", err)
		return ""
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		t.Errorf("file param %v is not a file id string", v)
		return ""
	}
	return s
}

func TestGetGet(t *testing.T) {
	bot, client := setupTestEnv(t)

	// Usage message.
	if err := GetGet(bot, testContext(bot, "/get", testUser(42))); err != nil {
		t.Fatalf("GetGet (usage): %v", err)
	}
	texts := client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "Пример использования") {
		t.Errorf("GetGet (usage) sent %q, want usage message", texts)
	}

	// Unknown get.
	client.Calls = nil
	if err := GetGet(bot, testContext(bot, "/get missing", testUser(42))); err != nil {
		t.Fatalf("GetGet (missing): %v", err)
	}
	texts = client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "не найден") {
		t.Errorf("GetGet (missing) sent %q, want not-found message", texts)
	}

	// Text get -> reply with the stored text.
	client.Calls = nil
	DB.Create(&Get{Name: "foo", Title: "foo", Type: "Text", Data: "привет", Creator: 100})
	if err := GetGet(bot, testContext(bot, "/get foo", testUser(42))); err != nil {
		t.Fatalf("GetGet (text): %v", err)
	}
	texts = client.sentTexts()
	if len(texts) != 1 || texts[0] != "привет" {
		t.Errorf("GetGet (text) sent %q, want %q", texts, "привет")
	}

	// Photo get -> sendPhoto with the stored file id.
	client.Calls = nil
	DB.Create(&Get{Name: "pic", Title: "pic", Type: "Photo", Data: "fileid123", Creator: 100})
	if err := GetGet(bot, testContext(bot, "/get pic", testUser(42))); err != nil {
		t.Fatalf("GetGet (photo): %v", err)
	}
	photos := client.callsFor("sendPhoto")
	if len(photos) != 1 || paramFileID(t, photos[0].Params["photo"]) != "fileid123" {
		t.Errorf("GetGet (photo) calls = %v, want one sendPhoto with fileid123", photos)
	}

	// Unknown get type -> error message.
	client.Calls = nil
	DB.Create(&Get{Name: "weird", Title: "weird", Type: "Banana", Data: "x", Creator: 100})
	if err := GetGet(bot, testContext(bot, "/get weird", testUser(42))); err != nil {
		t.Fatalf("GetGet (bad type): %v", err)
	}
	texts = client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "не знаю тип") {
		t.Errorf("GetGet (bad type) sent %q, want unknown-type message", texts)
	}
}

func TestGetInline(t *testing.T) {
	bot, client := setupTestEnv(t)
	DB.Create(&Get{Name: "foo", Title: "foo", Type: "Text", Data: "привет", Creator: 100})
	DB.Create(&Get{Name: "foobar", Type: "Photo", Data: "fileid123", Creator: 100})
	DB.Create(&Get{Name: "other", Type: "Text", Data: "другое", Creator: 100})

	// Empty query -> answered with no results.
	if err := GetInline(bot, testInlineContext(bot, testUser(42), "")); err != nil {
		t.Fatalf("GetInline (empty): %v", err)
	}
	answers := client.callsFor("answerInlineQuery")
	if len(answers) != 1 {
		t.Fatalf("GetInline (empty): answerInlineQuery calls = %d, want 1", len(answers))
	}
	if results, ok := answers[0].Params["results"].([]gotgbot.InlineQueryResult); !ok || len(results) != 0 {
		t.Errorf("GetInline (empty) results = %v, want empty", answers[0].Params["results"])
	}

	// Prefix query -> matching gets only.
	client.Calls = nil
	if err := GetInline(bot, testInlineContext(bot, testUser(42), "foo")); err != nil {
		t.Fatalf("GetInline: %v", err)
	}
	answers = client.callsFor("answerInlineQuery")
	if len(answers) != 1 {
		t.Fatalf("GetInline: answerInlineQuery calls = %d, want 1", len(answers))
	}
	results, ok := answers[0].Params["results"].([]gotgbot.InlineQueryResult)
	if !ok || len(results) != 2 {
		t.Fatalf("GetInline results = %v, want 2 results", answers[0].Params["results"])
	}
}

// TestGetInlineTypes covers the inline results of every get type: each type is
// offered as the cached inline result the Bot API expects for it, with the
// stored file id in the matching field, and a type the bot cannot send leaves
// the result empty.
func TestGetInlineTypes(t *testing.T) {
	bot, client := setupTestEnv(t)

	types := []string{"Animation", "Audio", "Photo", "Video", "Voice", "Document", "Text", "Sticker"}
	for i, typ := range types {
		if err := DB.Create(&Get{
			Name:    fmt.Sprintf("typetest%d", i),
			Title:   fmt.Sprintf("название %d", i),
			Type:    typ,
			Data:    fmt.Sprintf("fileid%d", i),
			Caption: fmt.Sprintf("подпись %d", i),
			Creator: 100,
		}).Error; err != nil {
			t.Fatalf("seed %v get: %v", typ, err)
		}
	}

	if err := GetInline(bot, testInlineContext(bot, testUser(42), "typetest")); err != nil {
		t.Fatalf("GetInline (types): %v", err)
	}
	answers := client.callsFor("answerInlineQuery")
	if len(answers) != 1 {
		t.Fatalf("GetInline (types): answerInlineQuery calls = %d, want 1", len(answers))
	}
	results, ok := answers[0].Params["results"].([]gotgbot.InlineQueryResult)
	if !ok || len(results) != len(types) {
		t.Fatalf("GetInline (types) results = %v, want %d", answers[0].Params["results"], len(types))
	}

	for i, typ := range types {
		fileID := fmt.Sprintf("fileid%d", i)
		title := fmt.Sprintf("название %d", i)
		caption := fmt.Sprintf("подпись %d", i)

		switch r := results[i].(type) {
		case *gotgbot.InlineQueryResultCachedGif:
			if typ != "Animation" {
				t.Errorf("GetInline (%v): result = %T, want the cached gif", typ, results[i])
			}
			if r.GifFileId != fileID || r.Title != title || r.Caption != caption {
				t.Errorf("GetInline (%v) = %+v, want gif %v/%v/%v", typ, *r, fileID, title, caption)
			}
		case *gotgbot.InlineQueryResultCachedVoice:
			if typ != "Voice" {
				t.Errorf("GetInline (%v): result = %T, want the cached voice", typ, results[i])
			}
			if r.VoiceFileId != fileID || r.Title != title || r.Caption != caption {
				t.Errorf("GetInline (%v) = %+v, want voice %v/%v/%v", typ, *r, fileID, title, caption)
			}
		case *gotgbot.InlineQueryResultCachedPhoto:
			if typ != "Photo" {
				t.Errorf("GetInline (%v): result = %T, want the cached photo", typ, results[i])
			}
			if r.PhotoFileId != fileID || r.Title != title || r.Caption != caption {
				t.Errorf("GetInline (%v) = %+v, want photo %v/%v/%v", typ, *r, fileID, title, caption)
			}
		case *gotgbot.InlineQueryResultCachedVideo:
			if typ != "Video" {
				t.Errorf("GetInline (%v): result = %T, want the cached video", typ, results[i])
			}
			if r.VideoFileId != fileID || r.Title != title || r.Caption != caption {
				t.Errorf("GetInline (%v) = %+v, want video %v/%v/%v", typ, *r, fileID, title, caption)
			}
		case *gotgbot.InlineQueryResultCachedDocument:
			if typ != "Audio" && typ != "Document" {
				t.Errorf("GetInline (%v): result = %T, want the cached document", typ, results[i])
			}
			if r.DocumentFileId != fileID || r.Title != title {
				t.Errorf("GetInline (%v) = %+v, want document %v/%v", typ, *r, fileID, title)
			}
			// Audio carries the caption as the description, Document as the caption.
			if typ == "Audio" && (r.Caption != "" || r.Description != caption) {
				t.Errorf("GetInline (Audio) = %+v, want the caption as the description", *r)
			}
			if typ == "Document" && (r.Caption != caption || r.Description != caption) {
				t.Errorf("GetInline (Document) = %+v, want the caption in both fields", *r)
			}
		case *gotgbot.InlineQueryResultArticle:
			if typ != "Text" {
				t.Errorf("GetInline (%v): result = %T, want the article", typ, results[i])
			}
			content, ok := r.InputMessageContent.(*gotgbot.InputTextMessageContent)
			if !ok {
				t.Errorf("GetInline (%v): content = %T, want the text content", typ, r.InputMessageContent)
				continue
			}
			if content.MessageText != fileID || r.Title != title || r.Description != fileID {
				t.Errorf("GetInline (%v) = %+v/%+v, want the stored text as the content", typ, *r, *content)
			}
		case nil:
			if typ != "Sticker" {
				t.Errorf("GetInline (%v): result = nil, want a cached result", typ)
			}
		default:
			t.Errorf("GetInline (%v): result = %T, want a cached result", typ, results[i])
		}
	}
}

func TestSet(t *testing.T) {
	bot, client := setupTestEnv(t)

	// Usage message.
	if err := Set(bot, testContext(bot, "/set foo", testUser(42))); err != nil {
		t.Fatalf("Set (usage): %v", err)
	}
	texts := client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "Пример использования") {
		t.Errorf("Set (usage) sent %q, want usage message", texts)
	}

	// Text get from arguments: value must not include the get name.
	client.Calls = nil
	if err := Set(bot, testContext(bot, "/set foo bar baz", testUser(42))); err != nil {
		t.Fatalf("Set: %v", err)
	}
	var get Get
	if err := DB.First(&get, "name = ?", "foo").Error; err != nil {
		t.Fatalf("Set: get not saved: %v", err)
	}
	if get.Type != "Text" || get.Data != "bar baz" || get.Creator != 42 {
		t.Errorf("Set saved %+v, want Text/%q/creator 42", get, "bar baz")
	}
	texts = client.sentTexts()
	if len(texts) != 1 || texts[0] != "Гет <code>foo</code> сохранён как <code>Text</code>." {
		t.Errorf("Set sent %q, want save confirmation", texts)
	}

	// Text get from a replied-to message.
	client.Calls = nil
	replyTo := testMessage("некий текст", testUser(42))
	if err := Set(bot, testReplyContext(bot, "/set bar", testUser(42), replyTo)); err != nil {
		t.Fatalf("Set (reply): %v", err)
	}
	var barGet Get
	if err := DB.First(&barGet, "name = ?", "bar").Error; err != nil {
		t.Fatalf("Set (reply): get not saved: %v", err)
	}
	if barGet.Type != "Text" || barGet.Data != "некий текст" {
		t.Errorf("Set (reply) saved %+v, want Text with data %q", barGet, "некий текст")
	}

	// A get owned by someone else cannot be changed by a random user.
	client.Calls = nil
	seedUser(42, "testpupkin", "Тест")
	if err := Set(bot, testContext(bot, "/set foo stolen", testUser(55))); err != nil {
		t.Fatalf("Set (ownership): %v", err)
	}
	texts = client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "Данный гет могут изменять либо администраторы, либо") {
		t.Errorf("Set (ownership) sent %q, want ownership message", texts)
	}
	var owned Get
	DB.First(&owned, "name = ?", "foo")
	if owned.Data != "bar baz" {
		t.Errorf("Set (ownership): get was modified anyway: %+v", owned)
	}

	// Admins may overwrite any get.
	client.Calls = nil
	if err := Set(bot, testContext(bot, "/set foo admin value", testUser(100))); err != nil {
		t.Fatalf("Set (admin): %v", err)
	}
	var updated Get
	DB.First(&updated, "name = ?", "foo")
	if updated.Data != "admin value" || updated.Creator != 100 {
		t.Errorf("Set (admin) saved %+v, want data %q creator 100", updated, "admin value")
	}
}

func TestGetall(t *testing.T) {
	bot, client := setupTestEnv(t)

	// No gets -> empty reply.
	if err := Getall(bot, testContext(bot, "/getall", testUser(42))); err != nil {
		t.Fatalf("Getall (empty): %v", err)
	}
	texts := client.sentTexts()
	if len(texts) != 1 || texts[0] != "" {
		t.Errorf("Getall (empty) sent %q, want empty list", texts)
	}

	// Two gets -> comma separated list.
	client.Calls = nil
	DB.Create(&Get{Name: "aaa", Type: "Text", Data: "a", Creator: 100})
	DB.Create(&Get{Name: "bbb", Type: "Text", Data: "b", Creator: 100})
	if err := Getall(bot, testContext(bot, "/getall", testUser(42))); err != nil {
		t.Fatalf("Getall: %v", err)
	}
	texts = client.sentTexts()
	want := "Доступные геты: aaa, bbb"
	if len(texts) != 1 || texts[0] != want {
		t.Errorf("Getall sent %q, want %q", texts, want)
	}
}

func TestSetGetOwner(t *testing.T) {
	bot, client := setupTestEnv(t)

	// Non-admin -> Denied.
	replyTo := testMessage("сообщение", &targetUser)
	if err := SetGetOwner(bot, testReplyContext(bot, "/setgetowner foo", testUser(42), replyTo)); err != nil {
		t.Fatalf("SetGetOwner (non-admin): %v", err)
	}
	if texts := client.sentTexts(); len(texts) != 1 || texts[0] != "нет" {
		t.Errorf("SetGetOwner (non-admin) sent %q, want Nope text", texts)
	}

	// Usage message when not replying.
	client.Calls = nil
	if err := SetGetOwner(bot, testContext(bot, "/setgetowner foo", testUser(100))); err != nil {
		t.Fatalf("SetGetOwner (usage): %v", err)
	}
	texts := client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "Пример использования") {
		t.Errorf("SetGetOwner (usage) sent %q, want usage message", texts)
	}

	// Unknown get.
	client.Calls = nil
	if err := SetGetOwner(bot, testReplyContext(bot, "/setgetowner missing", testUser(100), replyTo)); err != nil {
		t.Fatalf("SetGetOwner (missing): %v", err)
	}
	texts = client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "не найден") {
		t.Errorf("SetGetOwner (missing) sent %q, want not-found message", texts)
	}

	// Success: ownership change must actually be persisted.
	client.Calls = nil
	DB.Create(&Get{Name: "foo", Type: "Text", Data: "bar", Creator: 42})
	if err := SetGetOwner(bot, testReplyContext(bot, "/setgetowner foo", testUser(100), replyTo)); err != nil {
		t.Fatalf("SetGetOwner: %v", err)
	}
	var get Get
	DB.First(&get, "name = ?", "foo")
	if get.Creator != 777 {
		t.Errorf("SetGetOwner: creator = %d, want 777 (change was not saved)", get.Creator)
	}
	texts = client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "Владелец гета <code>foo</code> изменён на") {
		t.Errorf("SetGetOwner sent %q, want ownership change message", texts)
	}
}

func TestAddBless(t *testing.T) {
	bot, client := setupTestEnv(t)

	// Non-admin -> animation easter egg, no bless added.
	if err := AddBless(bot, testContext(bot, "/addbless text", testUser(42))); err != nil {
		t.Fatalf("AddBless (non-admin): %v", err)
	}
	if n := len(client.callsFor("sendAnimation")); n != 1 {
		t.Errorf("AddBless (non-admin): sendAnimation calls = %d, want 1", n)
	}
	if n := len(client.callsFor("sendMessage")); n != 0 {
		t.Errorf("AddBless (non-admin): sendMessage calls = %d, want 0", n)
	}

	// Usage message.
	client.Calls = nil
	if err := AddBless(bot, testContext(bot, "/addbless", testUser(100))); err != nil {
		t.Fatalf("AddBless (usage): %v", err)
	}
	texts := client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "Пример использования") {
		t.Errorf("AddBless (usage) sent %q, want usage message", texts)
	}

	// From arguments.
	client.Calls = nil
	if err := AddBless(bot, testContext(bot, "/addbless bless me please", testUser(100))); err != nil {
		t.Fatalf("AddBless: %v", err)
	}
	var bless Bless
	if err := DB.First(&bless, "text = ?", "bless me please").Error; err != nil {
		t.Errorf("AddBless: bless not saved: %v", err)
	}
	texts = client.sentTexts()
	if len(texts) != 1 || texts[0] != "Bless добавлен как <code>bless me please</code>." {
		t.Errorf("AddBless sent %q, want add confirmation", texts)
	}

	// From a replied-to message.
	client.Calls = nil
	replyTo := testMessage("ответный bless", testUser(42))
	if err := AddBless(bot, testReplyContext(bot, "/addbless", testUser(100), replyTo)); err != nil {
		t.Fatalf("AddBless (reply): %v", err)
	}
	var blessReply Bless
	if err := DB.First(&blessReply, "text = ?", "ответный bless").Error; err != nil {
		t.Errorf("AddBless (reply): bless not saved: %v", err)
	}

	// Reply to a message without text.
	client.Calls = nil
	empty := testMessage("", testUser(42))
	if err := AddBless(bot, testReplyContext(bot, "/addbless", testUser(100), empty)); err != nil {
		t.Fatalf("AddBless (no text): %v", err)
	}
	texts = client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "не смог найти текст") {
		t.Errorf("AddBless (no text) sent %q, want missing-text message", texts)
	}

	// Too long.
	client.Calls = nil
	long := "/addbless " + strings.Repeat("а", 201)
	if err := AddBless(bot, testContext(bot, long, testUser(100))); err != nil {
		t.Fatalf("AddBless (long): %v", err)
	}
	texts = client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "длиннее 200 символов") {
		t.Errorf("AddBless (long) sent %q, want too-long message", texts)
	}
}

func TestAddNope(t *testing.T) {
	bot, client := setupTestEnv(t)

	// Non-admin -> Denied.
	if err := AddNope(bot, testContext(bot, "/addnope text", testUser(42))); err != nil {
		t.Fatalf("AddNope (non-admin): %v", err)
	}
	if texts := client.sentTexts(); len(texts) != 1 || texts[0] != "нет" {
		t.Errorf("AddNope (non-admin) sent %q, want Nope text", texts)
	}

	// Usage message.
	client.Calls = nil
	if err := AddNope(bot, testContext(bot, "/addnope", testUser(100))); err != nil {
		t.Fatalf("AddNope (usage): %v", err)
	}
	texts := client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "Пример использования") {
		t.Errorf("AddNope (usage) sent %q, want usage message", texts)
	}

	// From arguments.
	client.Calls = nil
	if err := AddNope(bot, testContext(bot, "/addnope nope me please", testUser(100))); err != nil {
		t.Fatalf("AddNope: %v", err)
	}
	var nope Nope
	if err := DB.First(&nope, "text = ?", "nope me please").Error; err != nil {
		t.Errorf("AddNope: nope not saved: %v", err)
	}
	texts = client.sentTexts()
	if len(texts) != 1 || texts[0] != "Nope добавлен как <code>nope me please</code>." {
		t.Errorf("AddNope sent %q, want add confirmation", texts)
	}

	// From a replied-to message: stored lowercased.
	client.Calls = nil
	replyTo := testMessage("НЕПЕТ ТЕКСТ", testUser(42))
	if err := AddNope(bot, testReplyContext(bot, "/addnope", testUser(100), replyTo)); err != nil {
		t.Fatalf("AddNope (reply): %v", err)
	}
	var nopeReply Nope
	if err := DB.First(&nopeReply, "text = ?", "непет текст").Error; err != nil {
		t.Errorf("AddNope (reply): lowercased nope not saved: %v", err)
	}
}
