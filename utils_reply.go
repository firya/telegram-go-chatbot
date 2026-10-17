package main

// Reply is how the commands answer the user; ReplyAndRemove answers with a message that is
// deleted after 30 seconds.

import (
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

func Reply(message string, context ext.Context) error {
	_, err := Bot.SendMessage(context.Message.Chat.Id, message, &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML, ReceiverUserId: context.EffectiveUser.Id, ReplyParameters: &gotgbot.ReplyParameters{MessageId: context.Message.MessageId, AllowSendingWithoutReply: true}})
	if err != nil {
		return err
	}
	return nil
}

func ReplyAndRemove(message string, context ext.Context) error {
	message += "\n\nЭто сообщение самоуничтожится через 30 секунд."
	sentMessage, err := Bot.SendMessage(context.Message.Chat.Id, message, &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML, ReceiverUserId: context.EffectiveUser.Id, ReplyParameters: &gotgbot.ReplyParameters{MessageId: context.Message.MessageId, AllowSendingWithoutReply: true}})
	if err != nil {
		return err
	}
	if context.Message == nil || context.Message.MessageId == 0 {
		return nil
	}
	go func(msgid int64) {
		time.Sleep(30 * time.Second)
		Bot.DeleteMessage(context.Message.Chat.Id, msgid, nil)
	}(sentMessage.MessageId)
	return nil
}
