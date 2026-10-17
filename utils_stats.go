package main

// The stat counters every message feeds, and the start-of-day timestamp they key on.

import (
	"time"

	"github.com/neonxp/StemmerRu"
)

func statsIncrease(statType int64, dayTimestamp int64, contextID int64) {
	if DB.Exec("UPDATE stats SET count = count + 1, last_update = ? WHERE context_id = ? AND stat_type = ? AND day_timestamp = ?;", time.Now().Local().Unix(), contextID, statType, dayTimestamp).RowsAffected == 0 {
		DB.Create(Stats{StatType: statType, DayTimestamp: dayTimestamp, ContextID: contextID, Count: 1, LastUpdate: time.Now().Local().Unix()})
	}
}

func getWordID(searchWord string) int64 {
	shortWord := StemmerRu.Stem(searchWord)
	wordResult := StatsWords{}
	if DB.Model(StatsWords{}).Select("id").Where("short_word = ?", shortWord).Find(&wordResult).RowsAffected == 0 {
		wordResult.ShortWord = shortWord
		wordResult.Word = searchWord
		DB.Create(&wordResult)
	}
	return wordResult.ID
}

func GetStartOfDay() int64 {
	unixTS := time.Now().Local().Unix()
	tm := time.Unix(unixTS, 0).In(time.Local)
	hour, minute, second := tm.Clock()

	return unixTS - int64(hour*3600+minute*60+second)
}
