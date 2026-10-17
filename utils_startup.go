package main

// The startup notice the sysadmin gets when the bot is up: the runtime facts
// that matter when a command goes silent — build, host, connection, Bot API,
// database, chats, moderation lists, tool versions.
//
// Nothing that authenticates anything belongs in the message. The bot token is
// not in it at all, and the database DSN, the proxy URL, the MTProto app hash
// and the API keys are reported with the credentials stripped or as presence
// ("set" / "not set").

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// startupReport builds the text of the startup notice for the sysadmin.
func startupReport(bot *gotgbot.Bot, connectionType string) string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	exeDir := "unknown"
	if ex, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(ex)
	}

	return fmt.Sprintf("<a href=\"tg://user?id=%v\">Bot</a> has finished starting up.\n\n<code>\n"+
		"@%v (%v) on %v\n"+
		"Started: %v, pid %v\n"+
		"Build: %v\n"+
		"Binary: %v, config %v\n"+
		"Connection: %v\n"+
		"Updates: %v\n"+
		"Bot API: %v, datadir %v\n"+
		"Database: %v\n"+
		"Chats: main %v, reserve %v, comments %v, channel %v, stream %v\n"+
		"Moderation: admins %v, moders %v, sysadmin %v, antispam %v msg per %vs\n"+
		"Tools: yt-dlp %v, ffmpeg %v, proxy %v, font %v\n"+
		"Keys: app id %v, app hash %v, currency key %v, AI %v, model %v, AI token %v\n"+
		"</code>",
		bot.Id,
		bot.Username, bot.Id, host,
		time.Now().Format("2006-01-02 15:04:05 MST"), os.Getpid(),
		buildInfo(),
		exeDir, envFileName(),
		connectionInfo(connectionType),
		joinList(Config.AllowedUpdates),
		dashIfEmpty(Config.BotApiUrl), dashIfEmpty(Config.BotApiDataDir), databaseInfo(),
		chatId(Config.Chat), chatId(Config.ReserveChat), chatId(Config.CommentChat),
		chatId(Config.Channel), chatId(Config.StreamChannel),
		len(Config.Admins), len(Config.Moders), chatId(Config.SysAdmin),
		Config.AntiSpamCount, Config.AntiSpamWindow,
		ytdlpGetVer(), ffmpegVersion(), redactProxy(Config.Proxy), dashIfEmpty(Config.FontPath),
		Config.AppID, presence(Config.AppHash), presence(Config.CurrencyKey),
		dashIfEmpty(Config.AIURL), dashIfEmpty(Config.AIModel), presence(Config.AIToken))
}

// connectionInfo describes how the bot gets its updates: polling needs no
// address, a webhook is defined by the listening address, the public URL and
// the connection limit.
func connectionInfo(connectionType string) string {
	if connectionType != "webhook" {
		return "polling"
	}
	return fmt.Sprintf("webhook: listen %v, url %v, max %v",
		dashIfEmpty(Config.Listen), dashIfEmpty(Config.EndpointPublicURL), Config.MaxConnections)
}

// databaseInfo names the driver, the DSN without its password, and how many
// tables the migration produced. gorm.DB reaches its Dialector through an
// embedded *Config, so an uninitialized DB has a nil Config, not a nil
// Dialector.
func databaseInfo() string {
	if DB.Config == nil || DB.Config.Dialector == nil {
		return "not initialized"
	}
	info := fmt.Sprintf("%v, %v", DB.Dialector.Name(), redactDSN(Config.DSN))
	if tables, err := DB.Migrator().GetTables(); err == nil {
		info = fmt.Sprintf("%v (%v tables)", info, len(tables))
	}
	return info
}

// envFileName is the file the config was loaded from, the same one ConfigInit
// reads: ZAVTRA_ENV_FILE when it is set, .env in the working directory
// otherwise.
func envFileName() string {
	if file := os.Getenv("ZAVTRA_ENV_FILE"); file != "" {
		return file
	}
	return DefaultEnvFile
}

// buildInfo is the commit the binary was built from (Go stamps the VCS
// revision into the binary), plus the toolchain and the platform it runs on.
func buildInfo() string {
	info := fmt.Sprintf("go %v, %v/%v", runtime.Version(), runtime.GOOS, runtime.GOARCH)
	build, ok := debug.ReadBuildInfo()
	if !ok {
		return info
	}
	revision, modified, date := "unknown", "unknown", "unknown"
	for _, setting := range build.Settings {
		switch setting.Key {
		case "vcs.revision":
			if len(setting.Value) > 8 {
				setting.Value = setting.Value[:8]
			}
			revision = setting.Value
		case "vcs.modified":
			modified = "clean"
			if setting.Value == "true" {
				modified = "dirty"
			}
		case "vcs.time":
			date = setting.Value
		}
	}
	return fmt.Sprintf("rev %v (%v, %v), %v", revision, modified, date, info)
}

// dsnPassword matches the password of a keyword style DSN ("host=x password=y").
var dsnPassword = regexp.MustCompile(`(?i)(\bpassword\s*=\s*)[^;\s]*`)

// redactDSN drops the password from a DSN: the user name of a URL DSN
// ("postgres://user:secret@host/db") is kept, the keyword form gets
// "password=***". A local path (sqlite) has no credentials to drop.
func redactDSN(dsn string) string {
	if dsn == "" {
		return "-"
	}
	if u, err := url.Parse(dsn); err == nil && u.Scheme != "" && u.Host != "" {
		if u.User != nil {
			u.User = url.User(u.User.Username())
		}
		return u.String()
	}
	return dsnPassword.ReplaceAllString(dsn, "${1}***")
}

// redactProxy keeps the host and port of a proxy and drops its password
// ("http://user:secret@proxy:3128" is reported as "http://user@proxy:3128").
func redactProxy(proxy string) string {
	if proxy == "" {
		return "-"
	}
	if u, err := url.Parse(proxy); err == nil && u.User != nil {
		u.User = url.User(u.User.Username())
		return u.String()
	}
	return proxy
}

// ffmpegVersion is the version line of the installed ffmpeg, "Unknown" when it
// is not on the machine: media conversion depends on it.
func ffmpegVersion() string {
	var stdout bytes.Buffer

	cmd := exec.Command("ffmpeg", "-version")
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "Unknown"
	}

	line := strings.TrimSpace(strings.SplitN(stdout.String(), "\n", 2)[0])
	if fields := strings.Fields(line); len(fields) >= 3 && fields[0] == "ffmpeg" && fields[1] == "version" {
		return fields[2]
	}
	if line == "" {
		return "Unknown"
	}
	return line
}

// chatId formats a chat id, "-" when the chat is not configured.
func chatId(id int64) string {
	if id == 0 {
		return "-"
	}
	return strconv.FormatInt(id, 10)
}

// presence reports that a secret is configured without revealing it.
func presence(value string) string {
	if strings.TrimSpace(value) == "" {
		return "not set"
	}
	return "set"
}

// dashIfEmpty formats an optional string value as "-" when it is not set.
func dashIfEmpty(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

// joinList formats a list of values, "-" when it is empty.
func joinList(values []string) string {
	if len(values) == 0 {
		return "-"
	}
	return strings.Join(values, ", ")
}
