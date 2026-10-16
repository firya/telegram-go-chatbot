package main

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

// anekdotLineJoin merges a <br> that splits a sentence in half; the other <br>
// stay line breaks.
var anekdotLineJoin = regexp.MustCompile(`([а-я])<br>([а-я])`)

// parseAnekdot extracts the joke from the anekdot.ru random page: the text is
// embedded in a JSON.parse('[\"…\",\"…"]') blob. A page without that marker
// (a blocked request or a changed format) is an error, not a panic.
func parseAnekdot(page string) (string, error) {
	_, body, found := strings.Cut(page, "JSON.parse('[\\\"")
	if !found {
		return "", fmt.Errorf("anekdot.ru: в ответе нет текста анекдота")
	}
	text, _, _ := strings.Cut(body, "\\\",\\\"")
	text = strings.ReplaceAll(text, "\\\\\\\"", "\"")
	text = anekdotLineJoin.ReplaceAllString(text, `$1 $2`)
	return strings.ReplaceAll(text, "<br>", "\n"), nil
}

// Send an anekdot in chat on /anekdot
func Anekdot(bot *gotgbot.Bot, context *ext.Context) error {
	httpResponse, err := httpClient().Get("https://www.anekdot.ru/rss/randomu.html")
	if err != nil {
		return err
	}
	defer func(Body io.ReadCloser) {
		Body.Close()
	}(httpResponse.Body)
	html, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		return err
	}

	text, err := parseAnekdot(string(html))
	if err != nil {
		return err
	}

	return Reply(text, *context)
}
