package main

import (
	"testing"
)

// TestDataBaseInit covers the failure paths of the database start. Production
// runs on PostgreSQL, and the bot has to report a bad DSN, not crash: a DSN
// pgx cannot parse, and a DSN whose server is not there.
func TestDataBaseInit(t *testing.T) {
	// pgx rejects the DSN itself.
	if _, err := DataBaseInit("postgres://user:pass@127.0.0.1:1/db?sslmode=bogus"); err == nil {
		t.Errorf("DataBaseInit (broken DSN): err = nil, want the DSN parse error")
	}

	// The DSN parses, the server answers nothing: the migration reports it.
	// gorm.Open is lazy, so the failure must come from AutoMigrate.
	if _, err := DataBaseInit("postgres://user:pass@127.0.0.1:1/db?sslmode=disable&connect_timeout=1"); err == nil {
		t.Errorf("DataBaseInit (no server): err = nil, want the connection error")
	}
}
