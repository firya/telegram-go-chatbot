package main

import (
	"context"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	tgmd "github.com/eekstunt/telegramify-markdown-go"
)

// TestToEntities converts markdown the way the AI answers are converted and
// checks the mapping onto the Bot API entity types. Offsets and lengths are
// UTF-16 code units, as Telegram counts them.
func TestToEntities(t *testing.T) {
	if got := toEntities(nil); len(got) != 0 {
		t.Errorf("toEntities(nil) = %+v, want an empty slice", got)
	}

	msg := tgmd.Convert("**жирный** и [ссылка](https://example.com)")
	ents := toEntities(msg.Entities)
	if len(ents) != len(msg.Entities) {
		t.Fatalf("toEntities returned %d entities, want %d", len(ents), len(msg.Entities))
	}

	var bold, link *gotgbot.MessageEntity
	for i := range ents {
		switch ents[i].Type {
		case "bold":
			bold = &ents[i]
		case "text_link":
			link = &ents[i]
		}
	}

	if bold == nil {
		t.Fatalf("no bold entity in %+v", ents)
	}
	if bold.Offset != 0 || bold.Length != 6 {
		t.Errorf("bold entity = offset %d length %d, want offset 0 length 6", bold.Offset, bold.Length)
	}

	if link == nil {
		t.Fatalf("no text_link entity in %+v", ents)
	}
	if link.Url != "https://example.com" {
		t.Errorf("link entity url = %q, want the link target", link.Url)
	}
	if link.Length != 6 {
		t.Errorf("link entity length = %d, want 6 for the link text", link.Length)
	}
}

// TestCheckToolRestrictions covers the gate the AI agent runs every tool call
// through: the user is taken from the context, admins and moderators pass, and
// everyone else is limited by the ai_admin_only_tools list from config.json.
func TestCheckToolRestrictions(t *testing.T) {
	setupTestEnv(t) // Config.Admins = [100], Config.Moders = [300]

	userCtx := func(id int64) context.Context {
		return context.WithValue(context.Background(), "tgUser", id)
	}

	// Without a user id in the context the call cannot be attributed.
	if _, err := CheckToolRestrictions(context.Background(), "ban", "{}"); err == nil ||
		!strings.Contains(err.Error(), "ошибка определения пользователя") {
		t.Errorf("CheckToolRestrictions (no user): err = %v, want the user detection error", err)
	}

	// A missing admin-only list is reported as a configuration error, not as a
	// silent allowance.
	Config.AIAdminOnlyTools = nil
	if _, err := CheckToolRestrictions(userCtx(42), "ban", "{}"); err == nil ||
		!strings.Contains(err.Error(), "ai_admin_only_tools") {
		t.Errorf("CheckToolRestrictions (no config): err = %v, want the configuration error", err)
	}

	// A regular user is refused the admin-only tools.
	Config.AIAdminOnlyTools = []string{"ban", "mute"}
	if _, err := CheckToolRestrictions(userCtx(42), "ban", "{}"); err == nil ||
		!strings.Contains(err.Error(), "нет прав") {
		t.Errorf("CheckToolRestrictions (regular user, ban): err = %v, want the permission error", err)
	}

	// ...and keeps the tools that are not restricted, with arguments untouched.
	if args, err := CheckToolRestrictions(userCtx(42), "pidor", "{}"); err != nil || args != "{}" {
		t.Errorf("CheckToolRestrictions (regular user, pidor): args = %q, err = %v, want the arguments", args, err)
	}

	// Admins and moderators may use the restricted tools.
	for _, id := range []int64{100, 300} {
		if args, err := CheckToolRestrictions(userCtx(id), "ban", "{}"); err != nil || args != "{}" {
			t.Errorf("CheckToolRestrictions (%d, ban): args = %q, err = %v, want the arguments", id, args, err)
		}
	}
}

// TestToPtr covers the helper that hands a value to the model schema as a
// pointer: the pointer addresses a copy, so the caller's value is not aliased.
func TestToPtr(t *testing.T) {
	text := "картинка"
	p := toPtr(text)
	if p == nil {
		t.Fatal("toPtr returned nil")
	}
	if *p != text {
		t.Errorf("*toPtr = %q, want %q", *p, text)
	}
	if p == &text {
		t.Error("toPtr aliases the argument, want a pointer to a copy")
	}
	*p = "изменено"
	if text != "картинка" {
		t.Errorf("the source became %q, want it untouched", text)
	}

	if n := toPtr(42); *n != 42 {
		t.Errorf("*toPtr = %v, want 42", *n)
	}
}
