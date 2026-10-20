package main

import (
	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

// Khaleesi replies with a deliberately softened imitation of the target text.
func Khaleesi(bot *gotgbot.Bot, context *ext.Context) error {
	if context.Message.ReplyToMessage == nil {
		return Reply("Пример использования:\n/khaleesi в ответ на сообщение.", *context)
	}
	textInput := context.Message.ReplyToMessage.Text

	if context.Message.ReplyToMessage.Text == "" {
		textInput = context.Message.ReplyToMessage.Caption
	}

	if textInput == "" {
		return Reply("Текст не найден в указанном сообщении.", *context)
	}

	_, err := bot.SendMessage(context.EffectiveChat.Id, khaleesiText(textInput), &gotgbot.SendMessageOpts{
		ReplyParameters: &gotgbot.ReplyParameters{
			MessageId:                context.Message.ReplyToMessage.MessageId,
			AllowSendingWithoutReply: true,
		},
	})
	if err != nil {
		return err
	}
	return Remove(bot, context)
}
