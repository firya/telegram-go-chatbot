package main

import (
	"encoding/json"
	"io"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

type AdviceResp struct {
	ID    int    `json:"id,omitempty"`
	Text  string `json:"text,omitempty"`
	Sound string `json:"sound,omitempty"`
}

func Advice(bot *gotgbot.Bot, context *ext.Context) error {
	httpResponse, err := httpClient().Get("http://fucking-great-advice.ru/api/random")
	if err != nil {
		return err
	}
	defer func(Body io.ReadCloser) {
		Body.Close()
	}(httpResponse.Body)

	var advice AdviceResp
	err = json.NewDecoder(httpResponse.Body).Decode(&advice)
	if err != nil {
		return err
	}

	return Reply(advice.Text, *context)
}
