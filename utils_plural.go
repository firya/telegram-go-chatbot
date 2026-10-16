package main

// golang.org/x/text/message has no plural rules for Russian: its language.Russian
// catalog carries no plural catalog, so message.NewPrinter(language.Russian)
// always picks the "other" form ("2 предупреждений", "3 раз", "1 смертей").
// The comment in the commands ("prt will replace fmt ... according plurals
// defined in utils package") describes what ruPlural does here.

// ruPlural returns the correct Russian noun form for a count:
//
//	ruPlural(1, "победа", "победы", "побед") // "победа"
//	ruPlural(2, "победа", "победы", "побед") // "победы"
//	ruPlural(5, "победа", "победы", "побед") // "побед"
//	ruPlural(11, "победа", "победы", "побед") // "побед"
//
// The "one" form is the form for 1 in the surrounding phrase, so pass the case
// that phrase needs: numerals after a preposition take the accusative
// ("на 1 минуту", "через 1 минуту", "помолчать 1 минуту"), which is why the
// duration sites pass "минуту" and not "минута".
func ruPlural[T int | int64](count T, one, few, many string) string {
	n := int64(count)
	if n < 0 {
		n = -n
	}
	// 11-14 (and 111-114, ...) always take the genitive plural.
	if tens := n % 100; tens >= 11 && tens <= 14 {
		return many
	}
	switch n % 10 {
	case 1:
		return one
	case 2, 3, 4:
		return few
	default:
		return many
	}
}
