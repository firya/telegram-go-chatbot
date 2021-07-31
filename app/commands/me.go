package commands

import (
	"fmt"
	"strings"

	"github.com/NexonSU/telegram-go-chatbot/app/utils"
	"gopkg.in/tucnak/telebot.v3"
)

//Send formatted text on /me
func Me(context telebot.Context) error {
	var err error
	if context.Chat().Username != utils.Config.Telegram.Chat && !utils.IsAdminOrModer(context.Sender().Username) {
		return err
	}
	var text = strings.Split(context.Text(), " ")
	if len(text) == 1 {
		err := context.Reply("Пример использования:\n<code>/me {делает что-то}</code>")
		if err != nil {
			return err
		}
		return err
	}
	err := utils.Bot.Delete(context.Message())
	if err != nil {
		return err
	}
	_, err = utils.Bot.Send(context.Chat(), fmt.Sprintf("<code>%v %v</code>", utils.UserFullName(context.Sender()), strings.Join(text[1:], " ")))
	if err != nil {
		return err
	}
	return err
}
