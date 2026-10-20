package main

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

// resetSuicideState zeroes the Blessing globals (burst counter / video cooldown)
// so each test starts from a clean state.
func resetSuicideState() {
	firstSuicide, lastSuicide, burst, lastVideoSent = 0, 0, 0, 0
}

// seedChatStats inserts a Stats row of type 3 (per-user activity) used by the
// ricochet/redemption/shotgun queries.
func seedChatStats(contextID int64) {
	if err := DB.Create(&Stats{ContextID: contextID, StatType: 3, DayTimestamp: 1, Count: 1, LastUpdate: time.Now().Unix()}).Error; err != nil {
		panic("seedChatStats: " + err.Error())
	}
}

// duelistDeaths reads the Duelist row for a user (0 when absent).
func duelistDeaths(t *testing.T, userID int64) int {
	t.Helper()
	var duelist Duelist
	if err := DB.First(&duelist, userID).Error; err != nil {
		return 0
	}
	return duelist.Deaths
}

// announcedSeconds converts the first "N минут" phrase of a kill/blessing
// message into the seconds the victim was muted for (N*60). The Russian
// printer groups thousands with a non-breaking space ("15 360 минут"),
// so the separators are stripped before reading the number.
func announcedSeconds(t *testing.T, text string) int64 {
	t.Helper()
	text = strings.ReplaceAll(text, "\u00a0", "")
	m := minutesRE.FindStringSubmatch(text)
	if m == nil {
		t.Fatalf("нет «N минут» в тексте %q", text)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatalf("нечитаемое число в %q: %v", m[0], err)
	}
	return int64(n) * 60
}

func TestBlessing(t *testing.T) {
	bot, client := setupTestEnv(t)
	resetSuicideState()

	// Regular member: message removed, user restricted, death recorded.
	if err := Blessing(bot, testContext(bot, "/blessing", testUser(42))); err != nil {
		t.Fatalf("Blessing: %v", err)
	}
	if n := len(client.callsFor("deleteMessage")); n != 1 {
		t.Errorf("Blessing: deleteMessage calls = %d, want 1", n)
	}
	calls := client.callsFor("restrictChatMember")
	if len(calls) != 1 || calls[0].Params["user_id"] != int64(42) {
		t.Fatalf("Blessing: restrictChatMember calls = %v, want one for user 42", calls)
	}
	if until, ok := calls[0].Params["until_date"].(int64); !ok || time.Until(time.Unix(until, 0)) < 10*time.Minute {
		t.Errorf("Blessing: until_date = %v, want at least 10 minutes out", calls[0].Params["until_date"])
	}
	texts := client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "💥") || !strings.Contains(texts[0], "Респавн через") {
		t.Errorf("Blessing sent %q, want suicide message", texts)
	}
	if len(texts) == 1 {
		assertMinutesDeclined(t, texts[0])
	}
	if deaths := duelistDeaths(t, 42); deaths != 1 {
		t.Errorf("Blessing: duelist deaths = %d, want 1", deaths)
	}
	if got := statTotal(t, 8, 42); got != 1 {
		t.Errorf("Blessing: death stats = %d, want 1", got)
	}
	// The victim's mute time (stat type 7) matches the announced minutes.
	want42 := announcedSeconds(t, texts[0])
	if got := muteSeconds(t, 42); got != want42 {
		t.Errorf("Blessing: mute seconds = %d, want %d", got, want42)
	}

	// Second blessing increments deaths.
	client.Calls = nil
	if err := Blessing(bot, testContext(bot, "/blessing", testUser(42))); err != nil {
		t.Fatalf("Blessing (second): %v", err)
	}
	if deaths := duelistDeaths(t, 42); deaths != 2 {
		t.Errorf("Blessing (second): duelist deaths = %d, want 2", deaths)
	}
	if got := statTotal(t, 8, 42); got != 2 {
		t.Errorf("Blessing (second): death stats = %d, want 2", got)
	}
	texts = client.sentTexts()
	if len(texts) == 0 {
		t.Fatalf("Blessing (second): no message sent")
	}
	want42 += announcedSeconds(t, texts[len(texts)-1])
	if got := muteSeconds(t, 42); got != want42 {
		t.Errorf("Blessing (second): mute seconds = %d, want %d", got, want42)
	}

	// Premium users get the "премиально" flavor text.
	client.Calls = nil
	premium := testUser(55)
	premium.IsPremium = true
	if err := Blessing(bot, testContext(bot, "/blessing", premium)); err != nil {
		t.Fatalf("Blessing (premium): %v", err)
	}
	texts = client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "премиально") {
		t.Errorf("Blessing (premium) sent %q, want premium flavor text", texts)
	}
	if got := muteSeconds(t, 55); got != announcedSeconds(t, texts[0]) {
		t.Errorf("Blessing (premium): mute seconds = %d, want the announced minutes", got)
	}
	if got := statTotal(t, 8, 55); got != 1 {
		t.Errorf("Blessing (premium): death stats = %d, want 1", got)
	}

	// SysAdmin: the bullet ricochets onto a regular member from the stats.
	// resetSuicideState keeps the burst counter below the video threshold so
	// the ricochet message is sent as text.
	client.Calls = nil
	resetSuicideState()
	seedChatStats(777)
	if err := Blessing(bot, testContext(bot, "/blessing", testUser(100))); err != nil {
		t.Fatalf("Blessing (sysadmin): %v", err)
	}
	calls = client.callsFor("restrictChatMember")
	if len(calls) != 1 || calls[0].Params["user_id"] != int64(777) {
		t.Fatalf("Blessing (sysadmin): restrictChatMember calls = %v, want one for user 777", calls)
	}
	assertUntilDate(t, calls[0].Params, time.Now().Add(time.Minute))
	texts = client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "Пуля отскакивает") {
		t.Errorf("Blessing (sysadmin) sent %q, want ricochet message", texts)
	}
	// The death belongs to the ricochet victim - the user who gets restricted
	// and announced - not to the sysadmin who fired.
	if deaths := duelistDeaths(t, 777); deaths != 1 {
		t.Errorf("Blessing (sysadmin): victim deaths = %d, want 1", deaths)
	}
	if deaths := duelistDeaths(t, 100); deaths != 0 {
		t.Errorf("Blessing (sysadmin): shooter deaths = %d, want 0", deaths)
	}
	// The ricochet victim (1 minute) owns the mute time, not the sysadmin.
	if got := muteSeconds(t, 777); got != 60 {
		t.Errorf("Blessing (sysadmin): victim mute seconds = %d, want 60", got)
	}
	if got := muteSeconds(t, 100); got != 0 {
		t.Errorf("Blessing (sysadmin): shooter mute seconds = %d, want 0", got)
	}
	if got := statTotal(t, 8, 777); got != 1 {
		t.Errorf("Blessing (sysadmin): victim death stats = %d, want 1", got)
	}
	if got := statTotal(t, 8, 100); got != 0 {
		t.Errorf("Blessing (sysadmin): shooter death stats = %d, want 0", got)
	}

	// Burst: the 4th blessing in a row sends the suicide video instead of text.
	client.Calls = nil
	resetSuicideState()
	for i := 0; i < 4; i++ {
		if err := Blessing(bot, testContext(bot, "/blessing", testUser(42))); err != nil {
			t.Fatalf("Blessing (burst %d): %v", i+1, err)
		}
	}
	if n := len(client.callsFor("sendMessage")); n != 3 {
		t.Errorf("Blessing (burst): sendMessage calls = %d, want 3", n)
	}
	if n := len(client.callsFor("sendVideo")); n != 1 {
		t.Errorf("Blessing (burst): sendVideo calls = %d, want 1", n)
	}
}

func TestKill(t *testing.T) {
	bot, client := setupTestEnv(t)

	// Non-admin -> Denied.
	if err := Kill(bot, testContext(bot, "/kill 777", testUser(42))); err != nil {
		t.Fatalf("Kill (non-admin): %v", err)
	}
	if texts := client.sentTexts(); len(texts) != 1 || texts[0] != "нет" {
		t.Errorf("Kill (non-admin) sent %q, want Nope text", texts)
	}

	// Usage message.
	client.Calls = nil
	if err := Kill(bot, testContext(bot, "/kill", testUser(100))); err != nil {
		t.Fatalf("Kill (usage): %v", err)
	}
	texts := client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "Пример использования") {
		t.Errorf("Kill (usage) sent %q, want usage message", texts)
	}

	// Kill via reply: the command message is deleted, target restricted.
	client.Calls = nil
	seedUser(777, "target", "Цель")
	replyTo := testMessage("сообщение", &targetUser)
	if err := Kill(bot, testReplyContext(bot, "/kill", testUser(100), replyTo)); err != nil {
		t.Fatalf("Kill (reply): %v", err)
	}
	if n := len(client.callsFor("deleteMessage")); n != 1 {
		t.Errorf("Kill (reply): deleteMessage calls = %d, want 1", n)
	}
	calls := client.callsFor("restrictChatMember")
	if len(calls) != 1 || calls[0].Params["user_id"] != int64(777) {
		t.Fatalf("Kill (reply): restrictChatMember calls = %v, want one for user 777", calls)
	}
	texts = client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "пристрелил") {
		t.Errorf("Kill (reply) sent %q, want kill message", texts)
	}
	if len(texts) == 1 {
		assertMinutesDeclined(t, texts[0])
	}
	if deaths := duelistDeaths(t, 777); deaths != 1 {
		t.Errorf("Kill (reply): victim deaths = %d, want 1", deaths)
	}
	// Deaths are duplicated into the stats (stat type 8) for the /stats tops.
	if got := statTotal(t, 8, 777); got != 1 {
		t.Errorf("Kill (reply): death stats = %d, want 1", got)
	}
	// The victim's mute time (stat type 7) matches the announced minutes.
	want := announcedSeconds(t, texts[0])
	if got := muteSeconds(t, 777); got != want {
		t.Errorf("Kill (reply): mute seconds = %d, want %d", got, want)
	}

	// Kill by argument: no message deletion.
	client.Calls = nil
	if err := Kill(bot, testContext(bot, "/kill 777", testUser(100))); err != nil {
		t.Fatalf("Kill (arg): %v", err)
	}
	if n := len(client.callsFor("deleteMessage")); n != 0 {
		t.Errorf("Kill (arg): deleteMessage calls = %d, want 0", n)
	}
	if deaths := duelistDeaths(t, 777); deaths != 2 {
		t.Errorf("Kill (arg): victim deaths = %d, want 2", deaths)
	}
	if got := statTotal(t, 8, 777); got != 2 {
		t.Errorf("Kill (arg): death stats = %d, want 2", got)
	}
	texts = client.sentTexts()
	if len(texts) == 0 {
		t.Fatalf("Kill (arg): no message sent")
	}
	want += announcedSeconds(t, texts[len(texts)-1])
	if got := muteSeconds(t, 777); got != want {
		t.Errorf("Kill (arg): mute seconds = %d, want %d", got, want)
	}

	// /bless variant uses the "shush" wording.
	client.Calls = nil
	if err := Kill(bot, testContext(bot, "/bless 777", testUser(100))); err != nil {
		t.Fatalf("Kill (/bless): %v", err)
	}
	texts = client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "попросил") || !strings.Contains(texts[0], "помолчать") {
		t.Errorf("Kill (/bless) sent %q, want shush message", texts)
	}
	if len(texts) == 1 {
		assertMinutesDeclined(t, texts[0])
		want += announcedSeconds(t, texts[0])
		if got := muteSeconds(t, 777); got != want {
			t.Errorf("Kill (/bless): mute seconds = %d, want %d", got, want)
		}
	}
	if got := statTotal(t, 8, 777); got != 3 {
		t.Errorf("Kill (/bless): death stats = %d, want 3", got)
	}

	// Admin targets make the bullet ricochet onto a regular member.
	client.Calls = nil
	seedUser(300, "moder", "Модер")
	client.MemberStatus = map[int64]string{300: "administrator"}
	seedChatStats(777)
	if err := Kill(bot, testContext(bot, "/kill 300", testUser(100))); err != nil {
		t.Fatalf("Kill (ricochet): %v", err)
	}
	calls = client.callsFor("restrictChatMember")
	if len(calls) != 1 || calls[0].Params["user_id"] != int64(777) {
		t.Fatalf("Kill (ricochet): restrictChatMember calls = %v, want one for user 777", calls)
	}
	assertUntilDate(t, calls[0].Params, time.Now().Add(time.Minute))
	texts = client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "Пуля отскакивает") {
		t.Errorf("Kill (ricochet) sent %q, want ricochet message", texts)
	}
	if len(texts) == 1 {
		assertMinutesDeclined(t, texts[0])
	}
	// The ricochet victim (1 minute) owns the mute time, not the admin target.
	want += 60
	if got := muteSeconds(t, 777); got != want {
		t.Errorf("Kill (ricochet): mute seconds = %d, want %d", got, want)
	}
	if got := muteSeconds(t, 300); got != 0 {
		t.Errorf("Kill (ricochet): admin target mute seconds = %d, want 0", got)
	}
	// The ricochet death belongs to the victim, not to the admin target.
	if got := statTotal(t, 8, 777); got != 4 {
		t.Errorf("Kill (ricochet): victim death stats = %d, want 4", got)
	}
	if got := statTotal(t, 8, 300); got != 0 {
		t.Errorf("Kill (ricochet): admin target death stats = %d, want 0", got)
	}

	// kilo variants multiply the duration massively.
	client.Calls = nil
	client.MemberStatus = nil
	if err := Kill(bot, testContext(bot, "/kilo 777", testUser(100))); err != nil {
		t.Fatalf("Kill (kilo): %v", err)
	}
	calls = client.callsFor("restrictChatMember")
	if len(calls) != 1 {
		t.Fatalf("Kill (kilo): restrictChatMember calls = %d, want 1", len(calls))
	}
	if until, ok := calls[0].Params["until_date"].(int64); !ok || time.Until(time.Unix(until, 0)) < 24*time.Hour {
		t.Errorf("Kill (kilo): until_date = %v, want more than a day out", calls[0].Params["until_date"])
	}
	texts = client.sentTexts()
	if len(texts) == 0 {
		t.Fatalf("Kill (kilo): no message sent")
	}
	want += announcedSeconds(t, texts[len(texts)-1])
	if got := muteSeconds(t, 777); got != want {
		t.Errorf("Kill (kilo): mute seconds = %d, want %d", got, want)
	}
	if got := statTotal(t, 8, 777); got != 5 {
		t.Errorf("Kill (kilo): death stats = %d, want 5", got)
	}
}

func TestRedemption(t *testing.T) {
	bot, client := setupTestEnv(t)

	// Non-admin -> Denied.
	if err := Redemption(bot, testContext(bot, "/redemption", testUser(42))); err != nil {
		t.Fatalf("Redemption (non-admin): %v", err)
	}
	if texts := client.sentTexts(); len(texts) != 1 || texts[0] != "нет" {
		t.Errorf("Redemption (non-admin) sent %q, want Nope text", texts)
	}

	// No restricted users -> "nobody revived" text message.
	client.Calls = nil
	if err := Redemption(bot, testContext(bot, "/redemption", testUser(100))); err != nil {
		t.Fatalf("Redemption (empty): %v", err)
	}
	if n := len(client.callsFor("deleteMessage")); n != 1 {
		t.Errorf("Redemption (empty): deleteMessage calls = %d, want 1", n)
	}
	texts := client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "Но никто не воскрес") {
		t.Errorf("Redemption (empty) sent %q, want empty-redemption message", texts)
	}

	// Restricted users from the stats get full permissions back + video.
	client.Calls = nil
	seedChatStats(777)
	client.MemberStatus = map[int64]string{777: "restricted"}
	if err := Redemption(bot, testContext(bot, "/redemption", testUser(100))); err != nil {
		t.Fatalf("Redemption: %v", err)
	}
	calls := client.callsFor("restrictChatMember")
	if len(calls) != 1 || calls[0].Params["user_id"] != int64(777) {
		t.Fatalf("Redemption: restrictChatMember calls = %v, want one for user 777", calls)
	}
	videos := client.callsFor("sendVideo")
	if len(videos) != 1 {
		t.Fatalf("Redemption: sendVideo calls = %d, want 1", len(videos))
	}
	if caption, _ := videos[0].Params["caption"].(string); !strings.Contains(caption, "возродился") {
		t.Errorf("Redemption: video caption = %q, want revive text", caption)
	}
}

func TestShotgun(t *testing.T) {
	bot, client := setupTestEnv(t)

	// Non-admin -> Denied.
	if err := Shotgun(bot, testContext(bot, "/shotgun", testUser(42))); err != nil {
		t.Fatalf("Shotgun (non-admin): %v", err)
	}
	if texts := client.sentTexts(); len(texts) != 1 || texts[0] != "нет" {
		t.Errorf("Shotgun (non-admin) sent %q, want Nope text", texts)
	}

	// Two active members from the stats get restricted and recorded.
	client.Calls = nil
	seedChatStats(777)
	seedChatStats(778)
	if err := Shotgun(bot, testContext(bot, "/shotgun", testUser(100))); err != nil {
		t.Fatalf("Shotgun: %v", err)
	}
	if n := len(client.callsFor("restrictChatMember")); n != 2 {
		t.Errorf("Shotgun: restrictChatMember calls = %d, want 2", n)
	}
	texts := client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "выстрелил по чату из шотгана") {
		t.Errorf("Shotgun sent %q, want shotgun message", texts)
	}
	if deaths := duelistDeaths(t, 777); deaths != 1 {
		t.Errorf("Shotgun: victim 777 deaths = %d, want 1", deaths)
	}
	// Each victim was muted for a minute (stat type 7).
	if got := muteSeconds(t, 777); got != 60 {
		t.Errorf("Shotgun: victim 777 mute seconds = %d, want 60", got)
	}
	if got := muteSeconds(t, 778); got != 60 {
		t.Errorf("Shotgun: victim 778 mute seconds = %d, want 60", got)
	}
	if got := statTotal(t, 8, 777); got != 1 {
		t.Errorf("Shotgun: victim 777 death stats = %d, want 1", got)
	}
	if got := statTotal(t, 8, 778); got != 1 {
		t.Errorf("Shotgun: victim 778 death stats = %d, want 1", got)
	}

	// gigabite variant uses the bite wording.
	client.Calls = nil
	if err := Shotgun(bot, testContext(bot, "/gigabite", testUser(100))); err != nil {
		t.Fatalf("Shotgun (gigabite): %v", err)
	}
	texts = client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "сделал кусь чату") {
		t.Errorf("Shotgun (gigabite) sent %q, want bite message", texts)
	}

	// More than 10 candidates: only 10 victims are shot.
	client.Calls = nil
	DB.Where("stat_type = 3").Delete(&Stats{})
	for id := int64(701); id <= 712; id++ {
		seedChatStats(id)
	}
	if err := Shotgun(bot, testContext(bot, "/shotgun", testUser(100))); err != nil {
		t.Fatalf("Shotgun (cap): %v", err)
	}
	if n := len(client.callsFor("restrictChatMember")); n != 10 {
		t.Errorf("Shotgun (cap): restrictChatMember calls = %d, want 10", n)
	}
}

func TestBlessingTop(t *testing.T) {
	bot, client := setupTestEnv(t)

	// Empty top.
	if err := BlessingTop(bot, testContext(bot, "/blessingtop", testUser(42))); err != nil {
		t.Fatalf("BlessingTop (empty): %v", err)
	}
	texts := client.sentTexts()
	if len(texts) != 1 || texts[0] != "Топ-10 исекаев чата:\n\n" {
		t.Errorf("BlessingTop (empty) sent %q, want empty top", texts)
	}

	// Ranked duelists; known users are shown by name, unknown by id.
	client.Calls = nil
	seedUser(42, "test42", "Тест")
	DB.Create(&Duelist{UserID: 42, Deaths: 5})
	DB.Create(&Duelist{UserID: 777, Deaths: 3})
	if err := BlessingTop(bot, testContext(bot, "/blessingtop", testUser(42))); err != nil {
		t.Fatalf("BlessingTop: %v", err)
	}
	texts = client.sentTexts()
	if len(texts) != 1 || !strings.Contains(texts[0], "1. Тест - 5 раз") || !strings.Contains(texts[0], "2. 777 - 3 раз") {
		t.Errorf("BlessingTop sent %q, want ranked top", texts)
	}
}
