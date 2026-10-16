package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestRestart covers /restart. The admin path ends with os.Exit, so it runs in
// a subprocess: the child must stop inside the handler, which shows up as exit
// code 0 with no test report after the marker.
func TestRestart(t *testing.T) {
	if os.Getenv("ZAVTRA_RESTART_SUBPROCESS") == "1" {
		bot, _ := setupTestEnv(t)
		fmt.Println("restart-handler-started")
		Restart(bot, testContext(bot, "/restart", testUser(100)))
		fmt.Println("restart-handler-returned")
		return
	}

	bot, client := setupTestEnv(t)

	// A regular user gets the random refusal, the message stays, the bot lives.
	if err := Restart(bot, testContext(bot, "/restart", testUser(42))); err != nil {
		t.Fatalf("Restart (regular user): %v", err)
	}
	texts := client.sentTexts()
	if len(texts) != 1 || texts[0] != "нет" {
		t.Errorf("Restart (regular user) sent %q, want the refusal from the pool", texts)
	}
	if calls := client.callsFor("deleteMessage"); len(calls) != 0 {
		t.Errorf("Restart (regular user): deleteMessage calls = %d, want 0", len(calls))
	}

	// The admin path deletes the command message and exits the process.
	cmd := exec.Command(os.Args[0], "-test.run=^TestRestart$")
	cmd.Env = append(os.Environ(), "ZAVTRA_RESTART_SUBPROCESS=1")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("restart subprocess: %v\n%s", err, out)
	}
	output := string(out)
	if !strings.Contains(output, "restart-handler-started") {
		t.Fatalf("restart subprocess output = %q, want the handler to start", output)
	}
	if strings.Contains(output, "restart-handler-returned") || strings.Contains(output, "PASS") {
		t.Errorf("restart subprocess output = %q, want os.Exit inside the handler", output)
	}
}
