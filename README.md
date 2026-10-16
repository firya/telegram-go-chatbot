# zavtrachat_bot

**Русский** | [English](README.en.md)

Telegram-бот для одного конкретного чата на Go. Работает через Bot API (long polling или webhook,
поддерживается self-hosted Bot API server), дополнительно использует MTProto-клиент для нескольких
команд. Состояние — в PostgreSQL.

- 94 команды, зарегистрированные в `main.go` (часть — алиасы одной функции)
- 86 исходников `.go`, 141 тест в 22 файлах
- GitHub Actions: сборка + деплой на self-hosted раннере и публикация образа в `ghcr.io`
- Лицензия — MIT
- С 16.10.2026 разработку ведёт ИИ — модели локального стека Strata (`swift-1.5-iq3_xxs`,
  `qwen3.8-flash-next-iq3_xxs` и др., подробности — в разделе [Разработка](#разработка))

## Команды

### Модерация
| Команда | Что делает |
|---|---|
| `/ban`, `/unban` | бан / разбан, опционально время в секундах (`/ban @user 3600`) |
| `/mute`, `/unmute` | заглушить / разглушить, опционально время в секундах |
| `/kick` | кик (бан + разбан, чтобы можно было вернуться по ссылке) |
| `/warn`, `/mywarns` | предупреждение / сколько у меня предупреждений |
| `/remove` | удалить сообщение, ответом на которое послана команда |
| `/debug`, `/debuguser`, `/finduserinmessage` | JSON-дампы сообщения/юзера для отладки |

Доступ к модерационным командам — по спискам `admins` / `moders` / `sysadmin` в `config.json`.

### Игры и «социальное»
| Команда | Что делает |
|---|---|
| `/duel`, `/russianroulette` | вызов на дуэль (русская рулетка), кнопки принятия/отказа |
| `/duelstats` | своя статистика дуэлей |
| `/kill` и алиасы `bite`, `kilobite`, `kilomlem`, `megabite`, `megamlem`, `bless` | «пристрелить» юзера (временное ограничение) |
| `/revive`, `/resurrect`, `/redemption`, `/massrevive`, `/megarevive` | возродить одного / всех недавних |
| `/shotgun`, `gigabite`, `gigakill`, `ultrakill` | выстрел по чату |
| `/isekai`, `suicide`, `blessing`, `/isekaitop`, `/blessingtop` | «устроиться в роскомнадзор» и топ чата |
| `/addbless`, `/addnope` | добавить причину блесса / текст отказа по кнопке |
| `/pidor`, `/pidoreg`, `/pidorlist`, `/pidorme`, `/pidorstats`, `/pidorall`, `/pidorules`, `/pidordel`, `/pidorremovetoday` | игра «Пидор Дня!» |
| `/slap`, `/hug`, `/bonk`, `/marco`, `/shrug`, `/me` | стикеры/мем-действия и IRC-подобный `/me` |

### Медиа
| Команда | Что делает | Нужно |
|---|---|---|
| `/meow`, `/mlem`, `/woof` | гифка с котиком / не-котиком | MTProto |
| `/meme` | мем из канала | MTProto |
| `/convert` | конвертация медиа: `mp3`, `ogg`, `gif`, `audio`, `voice`, `animation` | ffmpeg, `bot_api_datadir` |
| `/distort`, `/invert`, `/reverse`, `/loop` | «переебать»/инвертировать/залупить медиа | ffmpeg, `bot_api_datadir` |
| `/mp3` | скачать аудио по ссылке (YouTube и т. п.) | yt-dlp, ffmpeg |
| `/download`, `/wget` | скачать видео по ссылке | yt-dlp |
| `/savetopm`, `/giveme`, `/topm` | сохранить пост в личку (копирует в PM, бот должен быть разблокирован) | — |

### Утилиты
| Команда | Что делает |
|---|---|
| `/get`, `/getall`, `/set`, `/del`, `/setgetowner` | «геты» — сохраняемые ответы бота (текст/медиа), инлайн-выбор через `@botname` |
| `/sed`, `/khaleesi`, `/say` | замена текста по образцу, мем-кхалиси, заставить бота сказать текст |
| `/stats` | статистика чата |
| `/advice`, `/anekdot`, `/releases` | совет, анекдот, список релизов игр |
| `/cur` | курсы валют (нужен `currency_key`) |
| `/google` | ссылка на поиск по тексту |
| `/date`, `/ping`, `/getid`, `/testrandom`, `/tldr` | дата сервера, понг, ID юзера, проверка рандома, подсказка про кнопку TLDR |
| `/restart` | `os.Exit(0)` — перезапуск бота (нужен `Restart=always` в systemd) |

### Обработчики без команд
- **`reserve_chat`** — удаляет системные сообщения о входе/выходе и добавляет юзеров в БД.
- **`channel`** — пересылает посты канала в чат; при заданном `stream_channel` дублирует туда.
- **Inline-режим** — `@botname <текст>` отдаёт сохранённые геты.
- **Реакции** в основном чате.
- **Глобальный middleware** — debug-лог каждого апдейта + антиспам: больше `antispam_count` сообщений
  за `antispam_window` секунд → предупреждение `sysadmin` в личку.

## Требования

| Зависимость | Где нужна |
|---|---|
| Go | 1.26+ (`go.mod`), в CI/`Dockerfile` база `golang:1.25` + автозамена тулчейна |
| PostgreSQL | обязателен, таблицы создаются `AutoMigrate` при старте |
| ffmpeg, ffprobe | для `/convert`, `/distort`, `/invert`, `/loop`, `/mp3` |
| yt-dlp | для `/mp3`, `/download` (в Docker-образе **не** установлен) |
| Self-hosted Bot API | опционально: `bot_api_url` (применяется ко всем API-вызовам); `bot_api_datadir` — чтобы `GetMedia` читал файлы напрямую с диска (для `/convert`, `/distort`, `/invert`, `/loop`) |
| Прямой доступ к датацентрам Telegram | для MTProto-команд: TCP на `149.154.*` / `91.108.*:443` + `app_id`/`app_hash` |

## Конфигурация

Бот читает `config.json` из текущей директории. Если файла нет — создаётся шаблон с правами `0600`.
Файл в `.gitignore`.

| Ключ | Назначение |
|---|---|
| `token` | токен бота — **обязателен** |
| `chat` | ID основного чата — **обязателен** |
| `dsn` | PostgreSQL DSN |
| `bot_api_url` | базовый URL Bot API (по умолчанию `https://api.telegram.org`) |
| `bot_api_datadir` | datadir self-hosted Bot API для прямого чтения файлов |
| `app_id`, `app_hash` | учётные данные MTProto-приложения для `/meow`, `/woof`, `/meme` |
| `allowed_updates` | какие апдейты запрашивать |
| `listen`, `endpoint_public_url`, `max_connections` | webhook-режим; если пусто — long polling |
| `reserve_chat` | чат, где удаляются сообщения о входе/выходе |
| `channel`, `stream_channel` | канал для пересылки постов и куда их дублировать |
| `admins`, `moders`, `sysadmin` | ID с правами модерации; `sysadmin` получает уведомления о старте, ошибках и антиспаме |
| `currency_key` | CoinMarketCap API key для `/cur` |
| `releases_url` | календарь для `/releases` (по умолчанию — PS/PS5/Switch/Xbox PAL) |
| `proxy` | HTTP-прокси для исходящих запросов: `/advice`, `/anekdot`, `/releases`, скачивание файлов (таймаут 10 с) |
| `antispam_window`, `antispam_count` | окно в секундах и порог сообщений для антиспама |
| `ai_url`, `ai_token`, `ai_model`, `ai_system`, `ai_admin_only_tools` | AI-модуль (`ai_*.go`), в `main.go` пока выключен |
| `fontpath`, `comment_chat` | объявлены в конфиге, в коде сейчас не используются |

## Запуск

```bash
go build -o zavtrachat_bot
./zavtrachat_bot            # из каталога, где лежит config.json
```

Docker:

```bash
docker build -t zavtrachat .
docker run -d --name zavtrachat -v "$PWD/config.json:/config.json" zavtrachat
```

В образе `WORKDIR /`, `ENTRYPOINT /goapp`, установлен ffmpeg; конфиг монтируется в `/config.json`.
`yt-dlp` в образ не входит — `/mp3` и `/download` в контейнере работать не будут.

CI (`.github/workflows/`):
- `deploy.yml` — на изменение `**.go`, `go.mod`/`go.sum` и самого workflow: `Test` гоняет `go vet` и
  `go test -race -count=1` на `ubuntu-latest` (покрытие складывается в артефакт), затем на `push` в
  `master` `Deploy` собирает `telegram-go-chatbot` на self-hosted раннере, копирует его в
  `/home/telegram/bot/` и делает `systemctl restart telegram-go-chatbot`. На pull request тесты
  идут, прод не перезапускается; параллельные деплои блокируются `concurrency`-группой;
- `docker-publish_yml` — сборка и публикация образа в `ghcr.io` + cosign-подпись. **Файл назван с
  `_yml` вместо `.yml`, поэтому GitHub Actions его не подхватывает** (workflows ищут `*.yml`/`*.yaml`).

## Тесты

```bash
go test -race -count=1 ./...                                   # 141 тест, ~20–40 с
PROTO_PROBE=1 go test -count=1 -run TestMtprotoConnectProbe -v .   # проверить доступ к датацентрам
```

Инфраструктура тестов — `testutils_test.go`:
- фейковый Bot API-клиент, записывающий все вызовы бота (проверяется отправленный текст/действия);
- in-memory SQLite (`github.com/glebarez/sqlite`, чистый Go, CGO не нужен) вместо PostgreSQL;
- хелперы `setupTestEnv`, `testMessage`, `testContext`, `testCallbackContext`, `seedUser`,
  `waitForCalls`.

Сети и Postgres тесты не требуют. Учитывайте: тесты идут на SQLite, прод — на PostgreSQL, поэтому
поведение БД в проде тестами не доказывается (`TestDataBaseInit` проверяет только пути отказа).
Зависимости от окружения: десять тестов гоняют настоящие `ffmpeg`/`ffprobe` по сгенерированным
секундным файлам и пропускаются, если бинарников нет (`TestFFmpegConvert`, `TestConvert`,
`TestInvert`, `TestDistort`, `TestLoopAnimation`, `TestDownloadFile`, `TestFFmpegRemoteMedia`,
`TestGetImageForLLM`, `TestGetImageForLLMRemote`, `TestAIWithPhoto`); `TestMtprotoConnectProbe`
пропускается без `PROTO_PROBE=1`, `TestYtdlpGetVer` — без установленного `yt-dlp`.

Наружу тесты не ходят: весь внешний трафик уходит в локальные заглушки. `mitm_test.go` поднимает
MITM-прокси — `TestMain` генерирует одноразовый CA, пишет его в `SSL_CERT_FILE` и публикует прокси
через `HTTP_PROXY`/`HTTPS_PROXY`, поэтому в заглушку попадают даже клиенты с дефолтным транспортом
(клиент CoinMarketCap); сертификаты для хостов подписываются на лету. Два пути перехвата:
`stubTLSProxy` (прокси для `httpClient()`) и `stubEnvRoute` (маршрут поверх прокси процесса).
Локальные `httptest`-серверы: заглушка Bot API (`TestBotInit` — `getMe`, удаление вебхука, polling
и отчёт сисадмину; `TestBotInitWebhook` — сервер вебхука поднимается до `setWebhook`, запрос без
secret-токена получает 401), OpenAI-формат для `/ai` (`TestAIStream`, `TestAIStreamError`,
`TestAIWithPhoto`). Особые приёмы: `TestRestart` запускает процесс заново (`os.Exit` внутри
обработчика нельзя проверить из того же процесса), `TestConfigInit` работает в `t.TempDir()`,
чтобы не трогать `config.json` чекаута.

## Структура

```
main.go              конфигурация обработчиков и запуск
utils_bot.go         инициализация бота, polling/webhook, middleware+антиспам
utils_bot_proto.go   MTProto-клиент: подключение, переподключение, таймауты
utils_config.go      Configuration + чтение config.json
utils_db.go          модели gorm и AutoMigrate
utils_common.go      общие хелперы: Reply, поиск юзера, статистика, реакции
utils_http.go        HTTP-клиент с прокси и таймаутом
utils_forward.go     пересылка постов канала
utils_files.go       go:embed стикеров (assets/*.png)
utils_plural.go      русские числительные (ruPlural)
command_*.go         команды
duel_*.go            дуэль: request/accept/stats
pidor_*.go           игра «Пидор Дня!»
bets_*.go            ставки
ai_*.go              AI-модуль (выключен)
*_test.go            тесты + harness
assets/              PNG стикеров (go:embed) и CREDITS.md про их лицензии
```

## MTProto-клиент

`/meow`, `/woof` и `/meme` работают через MTProto (`gotgproto` + `gotd`), а не через Bot API:

- клиент поднимается в фоновой горутине, подключение ограничено `protoConnectTimeout` (45 с),
  при неудаче — повтор каждые `protoReconnectDelay` (30 с); в логе видно `MTProto client connected`
  или причину (`MTProto client is not connected (attempt=N, error=…)`);
- команды при неподключённом клиенте отвечают «MTProto-клиент ещё не подключился», а не падают;
- на каждый запрос свой дедлайн (RPC 60 с, скачивание 3 мин), read-only запросы ретраятся;
- `NoUpdates: true` — бот не забирает апдейты у long polling Bot API, MTProto используется только
  для RPC;
- ключ `proxy` на MTProto **не влияет**: датацентры диалсятся прямым TCP, поэтому для этих команд
  нужен прямой доступ к `*:443` (или self-hosted MTProto-транзит).

## Известные особенности

- `proxy` применяется только к обычным HTTP-запросам бота (advice/anekdot/releases/скачивание
  файлов); вызовы Bot API и MTProto его не используют.
- `/restart` делает `os.Exit(0)` — перезапуск должен обеспечивать внешний супервизор (`Restart=always`
  в systemd, `restart: always` в compose).
- Ошибки хендлеров диспетчер отправляет в чат («Ошибка: …»), токен в тексте подменяется на `TOKEN`.
- AI-команды (`/ai`, `/restartai`, `/setaisystem`) закомментированы в `main.go`.

## Разработка

С **16 октября 2026 года** разработкой занимается ИИ: правки пишет модель, человек ставит задачи,
принимает решения и проверяет результат. Каждый такой коммит помечен в сообщении
(`Made with the … model.`) и соавтором (`Co-authored-by: … <noreply@opencode.ai>`), поэтому вклад ИИ
виден прямо в истории git.

Модели локального стека Strata — конфигурации в `~/.strata/*.json`, движок `~/.strata/engine/strata`,
веса в `~/Strata-data/models`:

| Модель (`model_name`) | Что это | В этом репозитории |
|---|---|---|
| `swift-1.5-iq3_xxs` | Swift-1.5 Qwen3.8-Flash-Next GSQ RCO, квант IQ3_XXS (`ukisai/Swift-1.5-Qwen3.8-Flash-Next-GSQ-RCO-GGUF`) | текущая: тест-харнес, фиксы MTProto/числительных, тесты команд, фиксы старта бота и `/ai`, тесты `config.json`/меню команд, README |
| `qwen3.8-flash-next-iq3_xxs` | Qwen3.8-Flash-Next GSQ RCO, квант IQ3_XXS (`ISTA-DASLab/Qwen3.8-Flash-Next-GSQ-RCO-GGUF`) | `e55e505` (go:embed), `30415bd` (тесты хелперов + фикс `RandInt`) |
| `qwen3.8-flash-next` | она же, до отдельного кванта | `4ab27c5` — первый ИИ-коммит (`bot_api_url` для всех API-вызовов) |
| `orcarouter-qwen3.8-flash-next-uncensored-iq3_xxs` | Qwen3.8-Flash-Next Uncensored, IQ3_XXS | настроена в `~/.strata`, к этому коду не применялась |

Рядом с ними в стеке: vision-проекции `mmproj-Qwen3.8-Flash-Next-BF16.gguf` и
`mmproj-Swift-Qwen3.8-Flash-Next-BF16.gguf`, черновик для спекулятивного декодирования
`mtp-q2_0.gguf`, а в `~/.strata/data/gguf_fingerprints.json` описаны и другие кванты этих же моделей
(Q2_0, IQ2_XS, IQ3_XXS, IQ3_S, coder IQ1_M, unsloth UD-IQ4_XS / UD-Q4_K_XL).

## Лицензия

MIT (см. `LICENSE`), © 2026 NexonSU. Лицензия покрывает исходники бота.

Что она не покрывает:

- `assets/bonk.png` и `assets/hug.png` — сторонние мемы, права на них не передаются; подробности в
  `assets/CREDITS.md`. При публичной публикации их стоит убрать или заменить на свои.
- `ffmpeg` и `yt-dlp` — внешние GPL-программы, бот вызывает их отдельным процессом; лицензия бота
  на них не распространяется, условия их лицензий — на их стороне.
- Почти все Go-зависимости — MIT / BSD-3 / Apache-2.0, они с MIT совместимы. Исключение —
  `github.com/celestix/gotgproto` под **GPL-3.0**: он линкуется в бинарник, поэтому при
  распространении собранного бота условия GPL-3 действуют на работу целиком (придётся раскрыть
  исходники). Для запуска бота на своём сервере обязательств нет: GPL срабатывает при
  распространении, а не при использовании.
