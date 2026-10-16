package main

import (
	"reflect"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

// channelPostContext builds an ext.Context for a channel post in Config.Channel.
func channelPostContext(bot *gotgbot.Bot, msg *gotgbot.Message) *ext.Context {
	msg.Chat = gotgbot.Chat{Id: Config.Channel, Type: "channel"}
	return ext.NewContext(bot, &gotgbot.Update{UpdateId: 1, ChannelPost: msg}, nil)
}

// TestForwardChannelPost covers the channel -> chat forwarding, including the
// album batching timer and the extra live-stream channel copy.
func TestForwardChannelPost(t *testing.T) {
	bot, client := setupTestEnv(t)
	// Set both channel ids up front: writing Config.StreamChannel later would
	// race with the album batching timer goroutine reading it in forwardMessages.
	Config.Channel = -400
	Config.StreamChannel = -500
	// Stop any batching timer before setupTestEnv restores the Bot global,
	// otherwise the timer callback would run against a nil Bot.
	t.Cleanup(func() {
		cache.mu.Lock()
		for groupID, timer := range cache.timers {
			timer.Stop()
			delete(cache.timers, groupID)
			delete(cache.groups, groupID)
		}
		cache.mu.Unlock()
	})

	// Posts outside the configured channel are ignored.
	other := testMessage("пост", nil)
	if err := ForwardChannelPost(bot, ext.NewContext(bot, &gotgbot.Update{UpdateId: 1, Message: other}, nil)); err != nil {
		t.Fatalf("ForwardChannelPost (other chat): %v", err)
	}
	if n := len(client.callsFor("forwardMessages")); n != 0 {
		t.Errorf("ForwardChannelPost (other chat): forwardMessages calls = %d, want 0", n)
	}

	// A single channel post is forwarded to the main chat immediately.
	client.Calls = nil
	post := testMessage("пост", nil)
	post.MessageId = 100
	if err := ForwardChannelPost(bot, channelPostContext(bot, post)); err != nil {
		t.Fatalf("ForwardChannelPost: %v", err)
	}
	calls := client.callsFor("forwardMessages")
	if len(calls) != 1 {
		t.Fatalf("ForwardChannelPost: forwardMessages calls = %d, want 1", len(calls))
	}
	if calls[0].Params["chat_id"] != int64(Config.Chat) || calls[0].Params["from_chat_id"] != int64(Config.Channel) {
		t.Errorf("ForwardChannelPost: chat_id = %v, from_chat_id = %v, want %v -> %v",
			calls[0].Params["chat_id"], calls[0].Params["from_chat_id"], Config.Chat, Config.Channel)
	}
	if ids, ok := calls[0].Params["message_ids"].([]int64); !ok || !reflect.DeepEqual(ids, []int64{100}) {
		t.Errorf("ForwardChannelPost: message_ids = %v, want [100]", calls[0].Params["message_ids"])
	}

	// An album is batched: the second post resets the timer, and the whole
	// group is forwarded once as sorted message ids. The post is a live one so
	// the timer goroutine makes a second API call after reading Config - waiting
	// for that call through the client mutex gives the race detector a
	// happens-before edge before setupTestEnv restores the Config global.
	client.Calls = nil
	first := testMessage("zavtracast/live альбом 1", nil)
	first.MessageId = 102
	first.MediaGroupId = "test-album-forward"
	second := testMessage("zavtracast/live альбом 2", nil)
	second.MessageId = 101
	second.MediaGroupId = "test-album-forward"
	if err := ForwardChannelPost(bot, channelPostContext(bot, first)); err != nil {
		t.Fatalf("ForwardChannelPost (album 1): %v", err)
	}
	if err := ForwardChannelPost(bot, channelPostContext(bot, second)); err != nil {
		t.Fatalf("ForwardChannelPost (album 2): %v", err)
	}
	if n := len(client.callsFor("forwardMessages")); n != 0 {
		t.Errorf("ForwardChannelPost (album): forwardMessages calls before the timer = %d, want 0", n)
	}
	waitForCalls(t, client, "forwardMessages", 2)
	calls = client.callsFor("forwardMessages")
	if ids, ok := calls[0].Params["message_ids"].([]int64); !ok || !reflect.DeepEqual(ids, []int64{101, 102}) {
		t.Errorf("ForwardChannelPost (album): message_ids = %v, want [101 102] sorted", calls[0].Params["message_ids"])
	}
	if calls[1].Params["chat_id"] != int64(Config.StreamChannel) {
		t.Errorf("ForwardChannelPost (album): stream forward chat_id = %v, want %v", calls[1].Params["chat_id"], Config.StreamChannel)
	}

	// A zavtracast live post is additionally forwarded to the stream channel.
	client.Calls = nil
	live := testMessage("zavtracast/live", nil)
	live.MessageId = 200
	if err := ForwardChannelPost(bot, channelPostContext(bot, live)); err != nil {
		t.Fatalf("ForwardChannelPost (live): %v", err)
	}
	calls = client.callsFor("forwardMessages")
	if len(calls) != 2 {
		t.Fatalf("ForwardChannelPost (live): forwardMessages calls = %d, want 2 (chat + stream channel)", len(calls))
	}
	if calls[1].Params["chat_id"] != int64(Config.StreamChannel) {
		t.Errorf("ForwardChannelPost (live): second forward chat_id = %v, want %v", calls[1].Params["chat_id"], Config.StreamChannel)
	}
}

// TestCacheAlbumId covers the album id caching helper used by SaveToPM.
func TestCacheAlbumId(t *testing.T) {
	bot, _ := setupTestEnv(t)
	t.Cleanup(func() {
		ChatAlbumCache.mu.Lock()
		delete(ChatAlbumCache.groups, "test-album-cache")
		ChatAlbumCache.mu.Unlock()
	})

	// Messages without a media group are not cached.
	if err := CacheAlbumId(bot, testContext(bot, "обычное сообщение", testUser(42))); err != nil {
		t.Fatalf("CacheAlbumId (no album): %v", err)
	}
	ChatAlbumCache.mu.Lock()
	n := len(ChatAlbumCache.groups)
	ChatAlbumCache.mu.Unlock()
	if n != 0 {
		t.Errorf("CacheAlbumId (no album): cached groups = %d, want 0", n)
	}

	// Album messages are appended to their media group.
	msg := testMessage("фото", testUser(42))
	msg.MessageId = 321
	msg.MediaGroupId = "test-album-cache"
	if err := CacheAlbumId(bot, ext.NewContext(bot, &gotgbot.Update{UpdateId: 1, Message: msg}, nil)); err != nil {
		t.Fatalf("CacheAlbumId: %v", err)
	}
	ChatAlbumCache.mu.Lock()
	ids := ChatAlbumCache.groups["test-album-cache"]
	ChatAlbumCache.mu.Unlock()
	if !reflect.DeepEqual(ids, []int64{321}) {
		t.Errorf("CacheAlbumId: cached ids = %v, want [321]", ids)
	}
}
