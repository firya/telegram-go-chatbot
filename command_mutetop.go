package main

import (
	"fmt"
	"strconv"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

// Send top 10 users by total mute time on /mutetop.
// Mute seconds are accumulated in the stats table as stat type 7
// (context_id = user id), keyed per day, so the top sums the days.
func MuteTop(bot *gotgbot.Bot, context *ext.Context) error {
	type muteTotal struct {
		ContextID int64
		Total     int64
	}
	var totals []muteTotal
	if err := DB.Model(&Stats{}).Where("stat_type = 7").
		Select("context_id", "SUM(count) AS total").
		Group("context_id").Order("total DESC").Limit(10).
		Scan(&totals).Error; err != nil {
		return err
	}
	text := "Топ-10 времени, проведённого в муте:\n\n"
	for i, row := range totals {
		name := strconv.FormatInt(row.ContextID, 10)
		if user, err := GetUserFromDB(name); err == nil {
			name = UserFullName(&user)
		}
		text += fmt.Sprintf("%v. %v - %v\n", i+1, name, formatDuration(row.Total))
	}
	_, err := context.Message.Reply(bot, text, &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML})
	return err
}
