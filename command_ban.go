package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

// Ban user on /ban
func Ban(bot *gotgbot.Bot, context *ext.Context) error {
	var untildate = time.Now().Unix() + 86400
	if !IsAdminOrModer(context.Message.From.Id) {
		return Denied(bot, context)
	}
	if (context.Message.ReplyToMessage == nil && len(context.Args()) == 1) || (context.Message.ReplyToMessage != nil && len(context.Args()) > 2) {
		return Reply("Пример использования: <code>/ban {ID или никнейм}</code>\nИли отправь в ответ на какое-либо сообщение <code>/ban</code>\nЕсли нужно забанить на время, то добавь время в секундах через пробел.", *context)
	}
	target, err := FindUserInMessage(*context.Message)
	// The duration may only come after the target: when replying, the target is
	// the replied-to message (so args[1] can be the time); when using arguments,
	// args[1] is the target lookup and args[2] the time.
	timeArg := 1
	if context.Message.ReplyToMessage == nil {
		timeArg = 2
	}
	if timeArg < len(context.Args()) {
		if addtime, parseErr := strconv.ParseInt(context.Args()[timeArg], 10, 64); parseErr == nil {
			untildate = time.Now().Unix() + addtime
		}
	}
	if err != nil {
		return err
	}
	result, err := bot.BanChatMember(context.Message.Chat.Id, target.Id, &gotgbot.BanChatMemberOpts{UntilDate: untildate})
	if err != nil {
		return err
	}
	if result {
		return Reply(fmt.Sprintf("Пользователь <a href=\"tg://user?id=%v\">%v</a> забанен%v.", target.Id, UserFullName(&target), RestrictionTimeMessage(untildate)), *context)
	} else {
		return Reply(fmt.Sprintf("Пользователь <a href=\"tg://user?id=%v\">%v</a> забанен%v.", target.Id, UserFullName(&target), RestrictionTimeMessage(untildate)), *context)
	}
}
