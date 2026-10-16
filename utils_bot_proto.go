package main

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/celestix/gotgproto"
	"github.com/celestix/gotgproto/sessionMaker"
	"github.com/gotd/contrib/middleware/floodwait"
	"github.com/gotd/contrib/middleware/ratelimit"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"golang.org/x/time/rate"
)

// The MTProto client is built in a background goroutine (main.go), so every access
// to it has to go through the lock below: handlers run as soon as the HTTP
// dispatcher is up, long before (or, if init failed, instead of) the client.
var (
	protoMu          sync.RWMutex
	GoTGProtoClient  *gotgproto.Client
	GoTGProtoContext context.Context
)

// mtprotoAPI returns the low level MTProto client together with its context.
// ok is false while the client is not connected, and callers must not touch it
// in that case.
func mtprotoAPI() (*tg.Client, context.Context, bool) {
	protoMu.RLock()
	defer protoMu.RUnlock()

	if GoTGProtoClient == nil || GoTGProtoContext == nil {
		return nil, nil, false
	}
	return GoTGProtoClient.API(), GoTGProtoContext, true
}

func setGoTGProtoClient(client *gotgproto.Client, ctx context.Context) {
	protoMu.Lock()
	GoTGProtoClient = client
	GoTGProtoContext = ctx
	protoMu.Unlock()
}

// MTProto talks to the Telegram datacenters over a direct TCP connection:
// bot_api_url and proxy are Bot API (HTTP) settings and do not apply here.
// When the datacenters are unreachable, gotgproto.NewClient blocks inside
// Start() retrying the connection forever, so a connect attempt gets its own
// deadline and is retried from the outside. Timeouts are variables so tests
// can shrink them.
var (
	protoConnectTimeout = 45 * time.Second
	protoReconnectDelay = 30 * time.Second
)

// MTProto requests get their own deadline. Without one a half-dead connection
// hangs the command handler forever, and floodwait is allowed to pause a
// request for a while, so the RPC timeout is deliberately generous.
var (
	protoRPCTimeout      = 60 * time.Second
	protoDownloadTimeout = 3 * time.Minute
)

// protoCall runs a single MTProto request with its own deadline, derived from
// the client context returned by mtprotoAPI.
func protoCall(base context.Context, fn func(ctx context.Context) error) error {
	callCtx, cancel := context.WithTimeout(base, protoRPCTimeout)
	defer cancel()
	return fn(callCtx)
}

// protoDownload runs a MTProto download with a longer deadline: a GIF is
// fetched in many chunks, so it outlives a plain RPC call.
func protoDownload(base context.Context, fn func(ctx context.Context) error) error {
	callCtx, cancel := context.WithTimeout(base, protoDownloadTimeout)
	defer cancel()
	return fn(callCtx)
}

// protoRetry runs a read-only MTProto request several times. Timeouts and
// connection errors on a flaky connection are transient, and these requests
// change nothing on the server, so repeating them is safe.
func protoRetry(base context.Context, attempts int, fn func(ctx context.Context) error) error {
	var err error
	for i := 0; i < attempts; i++ {
		if err = protoCall(base, fn); err == nil {
			return nil
		}
	}
	return err
}

// connectProtoClient builds and starts a MTProto client, giving the blocking
// handshake a deadline: gotgproto.NewClient retries the datacenter connection
// forever, so without one the init goroutine would hang in silence.
// On success the caller owns the returned stop function and must call it to
// release the connection; on failure it is already called here.
func connectProtoClient() (*gotgproto.Client, context.Context, context.CancelFunc, error) {
	baseCtx, cancel := context.WithCancel(context.Background())

	type connectResult struct {
		client *gotgproto.Client
		err    error
	}
	done := make(chan connectResult, 1)

	go func() {
		waiter := floodwait.NewWaiter().WithCallback(func(ctx context.Context, wait floodwait.FloodWait) {
			slog.Warn("MTProto flood wait", "duration", wait.Duration)
		})
		ratelimiter := ratelimit.New(rate.Every(time.Millisecond*100), 30)

		client, err := gotgproto.NewClient(
			Config.AppID,
			Config.AppHash,
			gotgproto.ClientTypeBot(Config.Token),
			&gotgproto.ClientOpts{
				Context: baseCtx,
				// Updates are consumed by the Bot API long poll; running the
				// MTProto update loop as well would steal them from it.
				NoUpdates:   true,
				InMemory:    true,
				Session:     sessionMaker.SimpleSession(),
				DialTimeout: 10 * time.Second,
				Middlewares: []telegram.Middleware{waiter, ratelimiter},
				RunMiddleware: func(origRun func(ctx context.Context, f func(ctx context.Context) error) (err error), ctx context.Context, f func(ctx context.Context) (err error)) (err error) {
					return origRun(ctx, func(ctx context.Context) error {
						return waiter.Run(ctx, f)
					})
				},
			},
		)
		done <- connectResult{client, err}
	}()

	select {
	case res := <-done:
		if res.err != nil {
			cancel()
			return nil, nil, nil, res.err
		}
		// baseCtx is the parent of the client's own context: cancelling it
		// here would stop the connection just established, so it is handed
		// to the caller instead.
		return res.client, res.client.CreateContext().Context, cancel, nil
	case <-time.After(protoConnectTimeout):
		cancel()
		return nil, nil, nil, fmt.Errorf("MTProto connect timed out after %v: the Telegram datacenters are unreachable (MTProto dials them directly, bot_api_url/proxy do not apply)", protoConnectTimeout)
	}
}

// gotdClientInit keeps the MTProto client connected for the commands that need
// it (/meow, /woof, /meme). It runs for the whole life of the process and never
// gives up: a datacenter that is briefly unreachable is retried, and the
// commands answer "not connected" meanwhile.
func gotdClientInit() {
	for attempt := 1; ; attempt++ {
		client, ctx, stop, err := connectProtoClient()
		if err != nil {
			slog.Error("MTProto client is not connected", "attempt", attempt, "error", err)
			time.Sleep(protoReconnectDelay)
			continue
		}

		setGoTGProtoClient(client, ctx)
		slog.Info("MTProto client connected")

		err = client.Idle()
		stop()
		setGoTGProtoClient(nil, nil)
		slog.Warn("MTProto client disconnected", "error", err)
		time.Sleep(protoReconnectDelay)
	}
}
