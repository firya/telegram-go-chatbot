package main

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/tg"
)

var woofChannel *tg.Channel

// Reply with GIF from Pan Kotek's channel
// The parameter is named ctx (not context) so the MTProto helpers below can
// use the context package.
func Woof(bot *gotgbot.Bot, ctx *ext.Context) error {
	api, protoCtx, ready := mtprotoAPI()
	if !ready {
		return Reply("MTProto-клиент ещё не подключился, попробуй чуть позже.", *ctx)
	}

	//get channel object
	if woofChannel == nil {
		var chats []tg.ChatClass
		err := protoRetry(protoCtx, 2, func(callCtx context.Context) error {
			channelResolve, err := api.ContactsResolveUsername(callCtx, &tg.ContactsResolveUsernameRequest{Username: "imnotacat"})
			if err != nil {
				return err
			}
			chats = channelResolve.GetChats()
			return nil
		})
		if err != nil {
			return err
		}
		if len(chats) == 0 {
			return fmt.Errorf("resolve @imnotacat: channel not found")
		}
		channel, ok := chats[0].(*tg.Channel)
		if !ok {
			return fmt.Errorf("resolve @imnotacat: not a channel")
		}
		woofChannel = channel
	}
	//prepare message query
	messagesQuery := []tg.InputMessageClass{}
	firstMessageId := RandInt(2, 4853)
	for message_id := firstMessageId; message_id < firstMessageId+10; message_id++ {
		messageObject := tg.Message{ID: message_id}
		messagesQuery = append(messagesQuery, messageObject.AsInputMessageID())
	}
	//query messages
	var messagesResult tg.MessagesMessagesClass
	err := protoRetry(protoCtx, 2, func(callCtx context.Context) error {
		var err error
		messagesResult, err = api.ChannelsGetMessages(callCtx, &tg.ChannelsGetMessagesRequest{
			Channel: woofChannel.AsInput(),
			ID:      messagesQuery,
		})
		return err
	})
	if err != nil {
		return err
	}
	channelMessages, ok := messagesResult.(*tg.MessagesChannelMessages)
	if !ok {
		return fmt.Errorf("get messages from @imnotacat: unexpected answer")
	}
	//search and download gif
	for _, mc := range channelMessages.Messages {
		if reflect.TypeOf(mc) != reflect.TypeOf(&tg.Message{}) {
			continue
		}
		messageMediaClass, check := mc.(*tg.Message).GetMedia()
		if check && reflect.TypeOf(messageMediaClass) == reflect.TypeOf(&tg.MessageMediaDocument{}) {
			document, check := messageMediaClass.(*tg.MessageMediaDocument).GetDocument()
			if !check {
				continue
			}
			docFile, check := document.AsNotEmpty()
			if !check {
				continue
			}
			fileNameObj, check := docFile.MapAttributes().AsDocumentAttributeFilename().First()
			filename := fileNameObj.FileName
			if !check {
				filename = strings.ReplaceAll(docFile.MimeType, "/", ".")
			}
			buf := bytes.Buffer{}
			if err := protoDownload(protoCtx, func(callCtx context.Context) error {
				_, err := downloader.NewDownloader().Download(api, docFile.AsInputDocumentFileLocation("")).Stream(callCtx, &buf)
				return err
			}); err != nil {
				return err
			}
			if strings.Contains(docFile.MimeType, "video") {
				if docFile.MimeType == "video/quicktime" {
					_, err = bot.SendAnimation(ctx.Message.Chat.Id, gotgbot.InputFileByReader(filename, &buf), &gotgbot.SendAnimationOpts{ParseMode: gotgbot.ParseModeHTML, ReplyParameters: &gotgbot.ReplyParameters{MessageId: ctx.Message.MessageId}})
				} else {
					_, err = bot.SendVideo(ctx.Message.Chat.Id, gotgbot.InputFileByReader(filename, &buf), &gotgbot.SendVideoOpts{ParseMode: gotgbot.ParseModeHTML, SupportsStreaming: true, ReplyParameters: &gotgbot.ReplyParameters{MessageId: ctx.Message.MessageId}})
				}
			}
			if strings.Contains(docFile.MimeType, "image") {
				_, err = bot.SendPhoto(ctx.Message.Chat.Id, gotgbot.InputFileByReader(filename, &buf), &gotgbot.SendPhotoOpts{ParseMode: gotgbot.ParseModeHTML, ReplyParameters: &gotgbot.ReplyParameters{MessageId: ctx.Message.MessageId}})
			}
			return err
		} else {
			continue
		}
	}

	return nil
}
