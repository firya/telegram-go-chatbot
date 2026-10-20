package main

import (
	"slices"
	"sort"
	"strings"
	"unicode"
)

type khaleesiRule struct {
	from      string
	to        string
	wordStart bool
	wordEnd   bool
	pattern   []rune
}

// Rules consume original runes only; replacements are not processed again.
// Longer combinations win, with declaration order breaking ties.
var khaleesiRules = makeKhaleesiRules([]khaleesiRule{
	// Context and endings, rather than exceptions for complete long words.
	{from: "нутр", to: "нуть"},
	{from: "арн", to: "яйн"},
	{from: "арна", to: "яйна"},
	{from: "холо", to: "хойо", wordStart: true},
	{from: "пел", to: "пил"},
	{from: "иль", to: "ий"},
	{from: "пёр", to: "пёй"},
	{from: "нап", to: "нап"},
	{from: "сток", to: "сток", wordEnd: true},
	{from: "анный", to: "ани", wordEnd: true},
	{from: "ерь", to: "ей"},
	{from: "арь", to: "ай"},
	{from: "ёр", to: "ёй"},
	{from: "зле", to: "зье"},
	{from: "зон", to: "зён"},
	{from: "трё", to: "тьё"},
	{from: "чн", to: "тьн"},
	{from: "ле", to: "ье"},
	{from: "сши", to: "сси"},
	{from: "фро", to: "фьё"},
	{from: "вка", to: "фькя"},
	{from: "кра", to: "кья"},
	{from: "лась", to: "ясь"},
	{from: "бота", to: "бётя"},
	{from: "чала", to: "тяла"},
	{from: "соо", to: "соо"},
	{from: "ишь", to: "ишь", wordEnd: true},
	{from: "ришь", to: "ишь", wordEnd: true},
	{from: "оришь", to: "оишь", wordEnd: true},

	// Consonant clusters.
	{from: "бро", to: "бьё"},
	{from: "про", to: "пьё"},
	{from: "пре", to: "пье"},
	{from: "при", to: "пьи"},
	{from: "пра", to: "пья"},
	{from: "пря", to: "пья"},
	{from: "фра", to: "фья"},
	{from: "тра", to: "тья"},
	{from: "тро", to: "тьё"},
	{from: "гро", to: "гё"},
	{from: "нра", to: "нья"},
	{from: "орь", to: "ёй"},
	{from: "ори", to: "ои"},
	{from: "ной", to: "нёй", wordEnd: true},
	{from: "лся", to: "лься"},
	{from: "тся", to: "ться", wordEnd: true},
	{from: "тч", to: "т"},
	{from: "дк", to: "тк"},
	{from: "нци", to: "ньци"},
	{from: "льш", to: "льс"},

	// Simplified р; the longer patterns above also consume its soft sign.
	{from: "рь", to: "й"},
	{from: "ри", to: "и"},
	{from: "ре", to: "е"},
	{from: "ря", to: "я"},
	{from: "ра", to: "я"},
	{from: "ро", to: "ё"},
	{from: "ру", to: "ю"},
	{from: "ры", to: "и"},
	{from: "ор", to: "ой"},
	{from: "ар", to: "ай"},
	{from: "ер", to: "ей"},
	{from: "рт", to: "йт"},

	// Hissing consonants; ш -> с applies before consonants too.
	{from: "чем", to: "тим"},
	{from: "ча", to: "тя"},
	{from: "чу", to: "тю"},
	{from: "че", to: "те"},
	{from: "чи", to: "ти"},
	{from: "чё", to: "тё"},
	{from: "ца", to: "тя"},
	{from: "цц", to: "тс"},
	{from: "щу", to: "щю"},
	{from: "шту", to: "штю"},
	{from: "щ", to: "с"},
	{from: "ш", to: "с"},
	{from: "ж", to: "з"},

	{from: "его", to: "ево"},
	{from: "ого", to: "ово"},
	{from: "со", to: "сё"},
	{from: "то", to: "тё"},
	{from: "те", to: "ти", wordEnd: true},
	{from: "ъ", to: "ь"},
})

// Whole-word substitutions are limited to short function words.
var khaleesiShortWords = map[string]string{
	"что":   "стё",
	"чтобы": "штоби",
	"со":    "со",
}

func makeKhaleesiRules(rules []khaleesiRule) []khaleesiRule {
	for i := range rules {
		rules[i].pattern = []rune(rules[i].from)
	}
	sort.SliceStable(rules, func(i, j int) bool {
		return len(rules[i].pattern) > len(rules[j].pattern)
	})
	return rules
}

// khaleesiText transforms Russian words and preserves all other characters.
func khaleesiText(text string) string {
	var result strings.Builder
	result.Grow(len(text))
	wordStart := -1
	for pos, r := range text {
		if isKhaleesiRussian(r) {
			if wordStart < 0 {
				wordStart = pos
			}
			continue
		}
		if wordStart >= 0 {
			result.WriteString(khaleesiWord(text[wordStart:pos]))
			wordStart = -1
		}
		result.WriteRune(r)
	}
	if wordStart >= 0 {
		result.WriteString(khaleesiWord(text[wordStart:]))
	}
	return result.String()
}

func khaleesiWord(original string) string {
	lower := strings.ToLower(original)
	if replacement, ok := khaleesiShortWords[lower]; ok {
		return khaleesiCase(original, replacement)
	}
	input := []rune(lower)
	var output strings.Builder
	for i := 0; i < len(input); {
		matched := false
		for _, rule := range khaleesiRules {
			end := i + len(rule.pattern)
			if end > len(input) || (rule.wordStart && i != 0) ||
				(rule.wordEnd && end != len(input)) {
				continue
			}
			if !slices.Equal(input[i:end], rule.pattern) {
				continue
			}
			output.WriteString(rule.to)
			i = end
			matched = true
			break
		}
		if matched {
			continue
		}
		r := input[i]
		if i > 0 && strings.ContainsRune("бвдзклмнпстфх", input[i-1]) {
			switch r {
			case 'а':
				r = 'я'
			case 'у':
				r = 'ю'
			case 'ы':
				r = 'и'
			}
		}
		output.WriteRune(r)
		i++
	}
	return khaleesiCase(original, output.String())
}

func isKhaleesiRussian(r rune) bool {
	return r >= 'а' && r <= 'я' || r >= 'А' && r <= 'Я' || r == 'ё' || r == 'Ё'
}

func khaleesiCase(original, transformed string) string {
	if original == strings.ToUpper(original) {
		return strings.ToUpper(transformed)
	}
	source := []rune(original)
	result := []rune(transformed)
	if len(result) > 0 && unicode.IsUpper(source[0]) {
		result[0] = unicode.ToUpper(result[0])
	}
	return string(result)
}
