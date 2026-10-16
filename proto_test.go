package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/celestix/gotgproto"
)

// The MTProto client is built in a background goroutine (main.go), so the
// commands that use it can run before it exists - and used to dereference the
// nil global (the /meow panic). They must answer the user instead.
func TestProtoCommandsWithoutClient(t *testing.T) {
	oldClient, oldCtx := GoTGProtoClient, GoTGProtoContext
	t.Cleanup(func() { setGoTGProtoClient(oldClient, oldCtx) })
	setGoTGProtoClient(nil, nil)

	commands := []struct {
		name     string
		response func(*gotgbot.Bot, *ext.Context) error
	}{
		{"meow", Meow},
		{"woof", Woof},
		{"meme", Meme},
	}

	for _, cmd := range commands {
		t.Run(cmd.name, func(t *testing.T) {
			bot, client := setupTestEnv(t)
			setGoTGProtoClient(nil, nil)

			ctx := testContext(bot, "/"+cmd.name, testUser(555))
			if err := cmd.response(bot, ctx); err != nil {
				t.Fatalf("%v: unexpected error: %v", cmd.name, err)
			}

			texts := client.sentTexts()
			if len(texts) != 1 {
				t.Fatalf("%v: expected a single reply, got %v", cmd.name, texts)
			}
			if !strings.Contains(texts[0], "не подключился") {
				t.Errorf("%v: expected a not-connected reply, got %q", cmd.name, texts[0])
			}
		})
	}
}

func TestMtprotoAPIAccessor(t *testing.T) {
	oldClient, oldCtx := GoTGProtoClient, GoTGProtoContext
	t.Cleanup(func() { setGoTGProtoClient(oldClient, oldCtx) })

	setGoTGProtoClient(nil, nil)
	if api, ctx, ok := mtprotoAPI(); ok {
		t.Errorf("expected not ready with nil client, got %v/%v", api, ctx)
	}

	// A client without its context is not usable either (init failed midway).
	setGoTGProtoClient(&gotgproto.Client{}, nil)
	if _, _, ok := mtprotoAPI(); ok {
		t.Error("expected not ready with nil context")
	}
}

// Init writes the globals from a goroutine while handlers read them; the
// accessors have to be safe for exactly that.
func TestMtprotoAPIAccessorConcurrent(t *testing.T) {
	oldClient, oldCtx := GoTGProtoClient, GoTGProtoContext
	t.Cleanup(func() { setGoTGProtoClient(oldClient, oldCtx) })

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				mtprotoAPI()
			}
		}()
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				setGoTGProtoClient(nil, nil)
			}
		}()
	}
	wg.Wait()
}

// A half-dead MTProto connection used to hang the command handlers: every
// request now runs under its own deadline.
func TestProtoCallTimeout(t *testing.T) {
	old := protoRPCTimeout
	t.Cleanup(func() { protoRPCTimeout = old })
	protoRPCTimeout = 30 * time.Millisecond

	start := time.Now()
	err := protoCall(context.Background(), func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("protoCall err = %v, want DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("protoCall took %v, want the short deadline", elapsed)
	}
}

// The request deadline is derived from the client context, so a client that
// goes away (Idle returned) cancels the in-flight requests too.
func TestProtoCallInheritsBaseContext(t *testing.T) {
	base, cancel := context.WithCancel(context.Background())
	cancel()

	err := protoCall(base, func(ctx context.Context) error {
		return ctx.Err()
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("protoCall err = %v, want Canceled", err)
	}
}

func TestProtoDownloadTimeout(t *testing.T) {
	old := protoDownloadTimeout
	t.Cleanup(func() { protoDownloadTimeout = old })
	protoDownloadTimeout = 250 * time.Millisecond

	err := protoDownload(context.Background(), func(ctx context.Context) error {
		deadline, ok := ctx.Deadline()
		if !ok {
			return errors.New("download request has no deadline")
		}
		if remaining := time.Until(deadline); remaining > protoDownloadTimeout || remaining <= 0 {
			return fmt.Errorf("unexpected deadline: %v", remaining)
		}
		return nil
	})
	if err != nil {
		t.Errorf("protoDownload: %v", err)
	}
}

// Read-only requests are repeated: timeouts and connection errors on a flaky
// connection are transient.
func TestProtoRetry(t *testing.T) {
	old := protoRPCTimeout
	t.Cleanup(func() { protoRPCTimeout = old })
	protoRPCTimeout = time.Second

	attempts := 0
	err := protoRetry(context.Background(), 3, func(ctx context.Context) error {
		attempts++
		if attempts < 2 {
			return errors.New("connection reset")
		}
		return nil
	})
	if err != nil {
		t.Errorf("protoRetry err = %v, want success on the second attempt", err)
	}
	if attempts != 2 {
		t.Errorf("protoRetry attempts = %d, want 2", attempts)
	}

	attempts = 0
	err = protoRetry(context.Background(), 2, func(ctx context.Context) error {
		attempts++
		return errors.New("boom")
	})
	if err == nil || err.Error() != "boom" {
		t.Errorf("protoRetry err = %v, want the last attempt's error", err)
	}
	if attempts != 2 {
		t.Errorf("protoRetry attempts = %d, want 2", attempts)
	}
}
