package main

// Reply is how the commands answer the user; ReplyAndRemove answers with a message that is
// deleted after 30 seconds; SysAdminReply delivers the operational notices (the startup
// report, handler errors, spam warnings) to the sysadmin.

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

// SysAdminReply sends an operational notice to the main chat as an ephemeral
// message: only the sysadmin of the config can read it, and Telegram destroys
// it after the chat's ephemeral validity, so nothing piles up in the chat.
// Without a main chat the notice falls back to the sysadmin's private chat,
// where ephemeral messages do not exist.
func SysAdminReply(message string) error {
	if Bot == nil || Config.SysAdmin == 0 {
		return nil
	}

	opts := &gotgbot.SendMessageOpts{ParseMode: gotgbot.ParseModeHTML}
	chatId := Config.Chat
	if chatId == 0 {
		chatId = Config.SysAdmin
	} else {
		opts.ReceiverUserId = Config.SysAdmin
	}

	_, err := Bot.SendMessage(chatId, message, opts)
	return err
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
