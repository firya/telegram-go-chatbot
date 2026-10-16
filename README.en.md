# zavtrachat_bot

[Русский](README.md) | **English**

A Go Telegram bot for one specific chat. It works through the Bot API (long polling or webhook,
self-hosted Bot API server supported) and additionally uses an MTProto client for a few commands.
State is kept in PostgreSQL.

- 94 commands registered in `main.go` (some of them are aliases of the same function)
- 86 `.go` source files, 99 tests in 13 files
- GitHub Actions: build + deploy on a self-hosted runner, image publishing to `ghcr.io`
- License — MIT
- Since 16.10.2026 development is driven by AI — models of the local Strata stack (`swift-1.5-iq3_xxs`,
  `qwen3.8-flash-next-iq3_xxs`, etc., details in the [Development](#development) section)

## Commands

### Moderation
| Command | What it does |
|---|---|
| `/ban`, `/unban` | ban / unban, optional duration in seconds (`/ban @user 3600`) |
| `/mute`, `/unmute` | mute / unmute, optional duration in seconds |
| `/kick` | kick (ban + unban, so the user can come back through an invite link) |
| `/warn`, `/mywarns` | warn somebody / how many warnings I have |
| `/remove` | delete the message the command was sent in reply to |
| `/debug`, `/debuguser`, `/finduserinmessage` | JSON dumps of a message/user for debugging |

Access to moderation commands comes from the `admins` / `moders` / `sysadmin` lists in `config.json`.

### Games and "social"
| Command | What it does |
|---|---|
| `/duel`, `/russianroulette` | challenge somebody to a duel (Russian roulette), accept/deny buttons |
| `/duelstats` | your own duel statistics |
| `/kill` and aliases `bite`, `kilobite`, `kilomlem`, `megabite`, `megamlem`, `bless` | "shoot" a user (temporary restriction) |
| `/revive`, `/resurrect`, `/redemption`, `/massrevive`, `/megarevive` | revive one user / all recent ones |
| `/shotgun`, `gigabite`, `gigakill`, `ultrakill` | shoot the whole chat |
| `/isekai`, `suicide`, `blessing`, `/isekaitop`, `/blessingtop` | "get hired by Roskomnadzor" and the chat leaderboard |
| `/addbless`, `/addnope` | add a blessing reason / a text shown when the duel button is declined |
| `/pidor`, `/pidoreg`, `/pidorlist`, `/pidorme`, `/pidorstats`, `/pidorall`, `/pidorules`, `/pidordel`, `/pidorremovetoday` | the "Pidor of the Day!" game |
| `/slap`, `/hug`, `/bonk`, `/marco`, `/shrug`, `/me` | stickers/meme actions and the IRC-style `/me` |

### Media
| Command | What it does | Needs |
|---|---|---|
| `/meow`, `/mlem`, `/woof` | a gif of a cat / a not-cat | MTProto |
| `/meme` | a meme from a channel | MTProto |
| `/convert` | media conversion: `mp3`, `ogg`, `gif`, `audio`, `voice`, `animation` | ffmpeg, `bot_api_datadir` |
| `/distort`, `/invert`, `/reverse`, `/loop` | mangle / invert / loop media | ffmpeg, `bot_api_datadir` |
| `/mp3` | download audio from a link (YouTube etc.) | yt-dlp, ffmpeg |
| `/download`, `/wget` | download video from a link | yt-dlp |
| `/savetopm`, `/giveme`, `/topm` | save a post to PM (copies it into the private chat, the bot must be unblocked) | — |

### Utilities
| Command | What it does |
|---|---|
| `/get`, `/getall`, `/set`, `/del`, `/setgetowner` | "gets" — saved bot answers (text/media), inline selection via `@botname` |
| `/sed`, `/khaleesi`, `/say` | pattern-based text replacement, meme-khaleesi, making the bot say a text |
| `/stats` | chat statistics |
| `/advice`, `/anekdot`, `/releases` | an advice, a joke, a list of game releases |
| `/cur` | currency rates (needs `currency_key`) |
| `/google` | a search link for the given text |
| `/date`, `/ping`, `/getid`, `/testrandom`, `/tldr` | server date, pong, user ID, randomness check, note about the TLDR button |
| `/restart` | `os.Exit(0)` — restarts the bot (needs `Restart=always` in systemd) |

### Handlers without commands
- **`reserve_chat`** — deletes join/leave system messages and adds users to the DB.
- **`channel`** — forwards channel posts into the chat; with `stream_channel` set, duplicates them there.
- **Inline mode** — `@botname <text>` returns the saved gets.
- **Reactions** in the main chat.
- **Global middleware** — debug log of every update + anti-spam: more than `antispam_count` messages
  within `antispam_window` seconds → a warning to `sysadmin` in PM.

## Requirements

| Dependency | Where it is needed |
|---|---|
| Go | 1.26+ (`go.mod`); CI/`Dockerfile` use `golang:1.25` as the base with toolchain auto-switch |
| PostgreSQL | required, tables are created by `AutoMigrate` at startup |
| ffmpeg, ffprobe | for `/convert`, `/distort`, `/invert`, `/loop`, `/mp3` |
| yt-dlp | for `/mp3`, `/download` (**not** installed in the Docker image) |
| Self-hosted Bot API | optional: `bot_api_url` (applied to every API call); `bot_api_datadir` so `GetMedia` reads files straight from disk (for `/convert`, `/distort`, `/invert`, `/loop`) |
| Direct access to Telegram datacenters | for the MTProto commands: TCP to `149.154.*` / `91.108.*:443` plus `app_id`/`app_hash` |

## Configuration

The bot reads `config.json` from the current directory. If the file is missing, a template is created
with `0600` permissions. The file is in `.gitignore`.

| Key | Purpose |
|---|---|
| `token` | bot token — **required** |
| `chat` | ID of the main chat — **required** |
| `dsn` | PostgreSQL DSN |
| `bot_api_url` | Bot API base URL (defaults to `https://api.telegram.org`) |
| `bot_api_datadir` | datadir of a self-hosted Bot API for reading files directly |
| `app_id`, `app_hash` | credentials of the MTProto app, used by `/meow`, `/woof`, `/meme` |
| `allowed_updates` | which updates to request |
| `listen`, `endpoint_public_url`, `max_connections` | webhook mode; if empty — long polling |
| `reserve_chat` | chat where join/leave messages are deleted |
| `channel`, `stream_channel` | channel to forward posts from, and where to duplicate them |
| `admins`, `moders`, `sysadmin` | IDs with moderation rights; `sysadmin` gets start, error and anti-spam notifications |
| `currency_key` | CoinMarketCap API key for `/cur` |
| `releases_url` | calendar used by `/releases` (defaults to a PS/PS5/Switch/Xbox PAL calendar) |
| `proxy` | HTTP proxy for outgoing requests: `/advice`, `/anekdot`, `/releases`, file downloads (10 s timeout) |
| `antispam_window`, `antispam_count` | window in seconds and message threshold for anti-spam |
| `ai_url`, `ai_token`, `ai_model`, `ai_system`, `ai_admin_only_tools` | AI module (`ai_*.go`), currently disabled in `main.go` |
| `fontpath`, `comment_chat` | declared in the config, currently unused in code |

## Running

```bash
go build -o zavtrachat_bot
./zavtrachat_bot            # from the directory that holds config.json
```

Docker:

```bash
docker build -t zavtrachat .
docker run -d --name zavtrachat -v "$PWD/config.json:/config.json" zavtrachat
```

The image has `WORKDIR /`, `ENTRYPOINT /goapp` and ffmpeg installed; the config is mounted at
`/config.json`. `yt-dlp` is not part of the image — `/mp3` and `/download` will not work in the container.

CI (`.github/workflows/`):
- `deploy.yml` — on `**.go` changes: builds `telegram-go-chatbot` on a self-hosted runner, copies it to
  `/home/telegram/bot/` and runs `systemctl restart telegram-go-chatbot`;
- `docker-publish_yml` — builds the image and publishes it to `ghcr.io` with a cosign signature.
  **The file is named `_yml` instead of `.yml`, so GitHub Actions never picks it up** (workflows are
  looked for as `*.yml`/`*.yaml`).

## Tests

```bash
go test -race -count=1 ./...                                   # 99 tests, ~15–30 s
PROTO_PROBE=1 go test -count=1 -run TestMtprotoConnectProbe -v .   # check datacenter access
```

The test infrastructure is `testutils_test.go`:
- a fake Bot API client that records every call the bot makes (the sent text/actions are asserted);
- in-memory SQLite (`github.com/glebarez/sqlite`, pure Go, no CGO) instead of PostgreSQL;
- helpers `setupTestEnv`, `testMessage`, `testContext`, `testCallbackContext`, `seedUser`,
  `waitForCalls`.

Tests need no network, no Postgres and no external binaries. Note that tests run on SQLite while
production runs on PostgreSQL, so production database behaviour is not proven by the tests.

## Layout

```
main.go              handler configuration and startup
utils_bot.go         bot init, polling/webhook, middleware + anti-spam
utils_bot_proto.go   MTProto client: connect, reconnect, timeouts
utils_config.go      Configuration + reading config.json
utils_db.go          gorm models and AutoMigrate
utils_common.go      shared helpers: Reply, user lookup, statistics, reactions
utils_http.go        HTTP client with proxy and timeout
utils_forward.go     forwarding channel posts
utils_files.go       go:embed of the sticker images (assets/*.png)
utils_plural.go      Russian numerals (ruPlural)
command_*.go         commands
duel_*.go            duel: request/accept/stats
pidor_*.go           the "Pidor of the Day!" game
bets_*.go            bets
ai_*.go              AI module (disabled)
*_test.go            tests + harness
assets/              sticker PNGs (go:embed) and CREDITS.md about their licensing
```

## MTProto client

`/meow`, `/woof` and `/meme` work through MTProto (`gotgproto` + `gotd`) rather than the Bot API:

- the client starts in a background goroutine, the connection attempt is bounded by
  `protoConnectTimeout` (45 s), on failure it retries every `protoReconnectDelay` (30 s); the log shows
  `MTProto client connected` or the reason (`MTProto client is not connected (attempt=N, error=…)`);
- with the client not connected, the commands answer "the MTProto client has not connected yet"
  instead of failing;
- every request has its own deadline (60 s for RPC, 3 min for downloads), read-only requests are retried;
- `NoUpdates: true` — the bot does not take updates away from the Bot API long polling, MTProto is used
  for RPC only;
- the `proxy` key does **not** affect MTProto: datacenters are spoken to over direct TCP, so these
  commands need direct access to `*:443` (or a self-hosted MTProto transit).

## Known quirks

- `proxy` applies only to the bot's ordinary HTTP requests (advice/anekdot/releases/file downloads);
  Bot API and MTProto calls do not use it.
- `/restart` calls `os.Exit(0)` — the restart must be handled by an external supervisor (`Restart=always`
  in systemd, `restart: always` in compose).
- The dispatcher sends handler errors to the chat ("Ошибка: …"), a token in the text is replaced with
  `TOKEN`.
- The AI commands (`/ai`, `/restartai`, `/setaisystem`) are commented out in `main.go`.

## Development

Since **16 October 2026** development is done by AI: a model writes the changes, a human sets the
tasks, makes the decisions and reviews the result. Every such commit is marked in its message
(`Made with the … model.`) and as a co-author (`Co-authored-by: … <noreply@opencode.ai>`), so the AI
contribution is visible directly in the git history.

Models of the local Strata stack — configurations in `~/.strata/*.json`, engine
`~/.strata/engine/strata`, weights in `~/Strata-data/models`:

| Model (`model_name`) | What it is | In this repository |
|---|---|---|
| `swift-1.5-iq3_xxs` | Swift-1.5 Qwen3.8-Flash-Next GSQ RCO, IQ3_XXS quant (`ukisai/Swift-1.5-Qwen3.8-Flash-Next-GSQ-RCO-GGUF`) | current one: test harness, MTProto/plural fixes, command tests, README |
| `qwen3.8-flash-next-iq3_xxs` | Qwen3.8-Flash-Next GSQ RCO, IQ3_XXS quant (`ISTA-DASLab/Qwen3.8-Flash-Next-GSQ-RCO-GGUF`) | `e55e505` (go:embed), `30415bd` (helper tests + `RandInt` fix) |
| `qwen3.8-flash-next` | the same model, before the separate quant | `4ab27c5` — the first AI commit (`bot_api_url` for all API calls) |
| `orcarouter-qwen3.8-flash-next-uncensored-iq3_xxs` | Qwen3.8-Flash-Next Uncensored, IQ3_XXS | configured in `~/.strata`, not applied to this code |

Alongside them the stack also has the vision projections `mmproj-Qwen3.8-Flash-Next-BF16.gguf` and
`mmproj-Swift-Qwen3.8-Flash-Next-BF16.gguf`, a draft model for speculative decoding `mtp-q2_0.gguf`,
and `~/.strata/data/gguf_fingerprints.json` describes other quantizations of the same models (Q2_0,
IQ2_XS, IQ3_XXS, IQ3_S, coder IQ1_M, unsloth UD-IQ4_XS / UD-Q4_K_XL).

## License

MIT (see `LICENSE`), © 2026 NexonSU. The license covers the source code of the bot.

What it does not cover:

- `assets/bonk.png` and `assets/hug.png` — third-party memes, their rights are not granted; details in
  [`assets/CREDITS.en.md`](assets/CREDITS.en.md). Before publishing the project, remove them or replace
  them with your own.
- `ffmpeg` and `yt-dlp` — external GPL programs the bot runs as separate processes; the bot's license
  does not extend to them, their own license terms apply.
- Almost all Go dependencies are MIT / BSD-3 / Apache-2.0 and compatible with MIT. The exception is
  `github.com/celestix/gotgproto` under **GPL-3.0**: it is linked into the binary, so distributing the
  built bot puts the whole work under GPL-3 (the sources have to be disclosed). Running the bot on your
  own server carries no obligation: GPL triggers on distribution, not on use.
