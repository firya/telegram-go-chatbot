package main

import (
	"context"
	"fmt"
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

// The MTProto client is created in a goroutine (see main.go), so every access
// to it has to go through the lock below: handlers run as soon as the HTTP
// dispatcher is up, long before (or, if init failed, instead of) the client.
var (
	protoMu          sync.RWMutex
	GoTGProtoClient  *gotgproto.Client
	GoTGProtoContext context.Context
)

// mtprotoAPI returns the low level MTProto client together with its context.
// ok is false while gotdClientInit has not finished yet (or has failed), and
// callers must not touch the client in that case.
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

// MTProto requests get their own deadline. Without one a half-dead connection
// hangs the command handler forever, and floodwait is allowed to pause a
// request for a while, so the RPC timeout is deliberately generous.
// They are variables so tests can shrink them.
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

func gotdClientInit() error {
	waiter := floodwait.NewWaiter().WithCallback(func(ctx context.Context, wait floodwait.FloodWait) {
		fmt.Printf("Waiting for flood, dur: %d\n", wait.Duration)
	})
	ratelimiter := ratelimit.New(rate.Every(time.Millisecond*100), 30)

	client, err := gotgproto.NewClient(
		Config.AppID,
		Config.AppHash,
		gotgproto.ClientTypeBot(Config.Token),
		&gotgproto.ClientOpts{
			InMemory:    true,
			Session:     sessionMaker.SimpleSession(),
			Middlewares: []telegram.Middleware{waiter, ratelimiter},
			RunMiddleware: func(origRun func(ctx context.Context, f func(ctx context.Context) error) (err error), ctx context.Context, f func(ctx context.Context) (err error)) (err error) {
				return origRun(ctx, func(ctx context.Context) error {
					return waiter.Run(ctx, f)
				})
			},
		},
	)
	if err != nil {
		return err
	}

	setGoTGProtoClient(client, client.CreateContext().Context)

	err = client.Idle()
	// The connection is gone: stop handing a dead client to the commands.
	setGoTGProtoClient(nil, nil)
	return err
}
