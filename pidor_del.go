package main

import (
	"fmt"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

// Remove user in DB on /pidordel
func Pidordel(bot *gotgbot.Bot, context *ext.Context) error {
	if !IsAdminOrModer(context.Message.From.Id) {
		return Denied(bot, context)
	}
	var pidor PidorList
	user, err := FindUserInMessage(*context.Message)
	if err != nil {
		return err
	}
	pidor = PidorList(user)
	result := DB.Delete(&pidor)
	if result.RowsAffected != 0 {
		return Reply(fmt.Sprintf("Пользователь %v удалён из игры <b>Пидор Дня</b>!", MentionUser(&user)), *context)
	} else {
		// gorm does not return an error when no rows matched - dereferencing result.Error here used to panic.
		errText := "пользователь не найден в игре"
		if result.Error != nil {
			errText = result.Error.Error()
		}
		return Reply(fmt.Sprintf("Не удалось удалить пользователя:\n<code>%v</code>", errText), *context)
	}
}
