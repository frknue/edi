// Command edi-telegram is the Life RPG's presence bot: a two-way Telegram
// companion that pushes a morning briefing and a (conditional) evening nudge
// and answers /status /quests /done /ward /rest. Like edi-cli and edi-mcp it
// is a thin HTTP client over the SAME REST API every other client uses — no
// DB access, no hidden endpoints.
//
// Environment:
//
//	TELEGRAM_BOT_TOKEN  required; from @BotFather
//	TELEGRAM_CHAT_ID    the one chat the bot serves; unset = pairing mode
//	                    (bot replies to /start with the chat id, nothing else)
//	EDI_API             backend base URL (default http://localhost:8080)
//	EDI_TOKEN           optional bearer token (matches the server's EDI_TOKEN)
//	EDI_BRIEFING_TIME   local HH:MM for the morning briefing (default 08:00)
//	EDI_NUDGE_TIME      local HH:MM for the evening nudge (default 20:00)
package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"edi/internal/apiclient"
	"edi/internal/telegram"
)

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN is required (create a bot with @BotFather)")
	}
	var chatID int64
	if raw := os.Getenv("TELEGRAM_CHAT_ID"); raw != "" {
		var err error
		if chatID, err = strconv.ParseInt(raw, 10, 64); err != nil {
			log.Fatalf("TELEGRAM_CHAT_ID %q is not a number", raw)
		}
	}

	api := apiclient.New(envOr("EDI_API", "http://localhost:8080"))
	api.Token = os.Getenv("EDI_TOKEN")
	tg := telegram.New(token)

	if chatID == 0 {
		log.Print("pairing mode: TELEGRAM_CHAT_ID unset — send /start to the bot to get your chat id")
		pairingLoop(tg)
		return
	}

	go pushLoop(api, tg, chatID, envOr("EDI_BRIEFING_TIME", "08:00"), sendBriefing, "briefing")
	go pushLoop(api, tg, chatID, envOr("EDI_NUDGE_TIME", "20:00"), sendNudge, "nudge")

	log.Printf("edi-telegram up: chat %d, api %s", chatID, api.BaseURL)
	pollLoop(api, tg, chatID)
}

// pairingLoop answers /start from ANY chat with that chat's id, so the user
// can discover the value for TELEGRAM_CHAT_ID. It does nothing else.
func pairingLoop(tg *telegram.Client) {
	var offset int64
	for {
		updates, err := tg.GetUpdates(offset, 50)
		if err != nil {
			log.Printf("getUpdates: %v (retrying in 5s)", err)
			time.Sleep(5 * time.Second)
			continue
		}
		for _, u := range updates {
			offset = u.UpdateID + 1
			if u.Message == nil {
				continue
			}
			cmd, _ := parseCommand(u.Message.Text)
			if cmd == "start" {
				msg := fmt.Sprintf("Your chat id is <b>%d</b>.\n\nRestart the bot with TELEGRAM_CHAT_ID=%d to pair it.",
					u.Message.Chat.ID, u.Message.Chat.ID)
				if err := tg.SendMessage(u.Message.Chat.ID, msg); err != nil {
					log.Printf("sendMessage: %v", err)
				}
			}
		}
	}
}

// pollLoop is the main long-poll cycle: only the configured chat is served;
// every other chat is silently ignored.
func pollLoop(api *apiclient.Client, tg *telegram.Client, chatID int64) {
	var offset int64
	for {
		updates, err := tg.GetUpdates(offset, 50)
		if err != nil {
			log.Printf("getUpdates: %v (retrying in 5s)", err)
			time.Sleep(5 * time.Second)
			continue
		}
		for _, u := range updates {
			offset = u.UpdateID + 1
			if u.Message == nil || u.Message.Chat.ID != chatID {
				continue
			}
			reply := handleCommand(api, u.Message.Text)
			if err := tg.SendMessage(chatID, reply); err != nil {
				log.Printf("sendMessage: %v", err)
			}
		}
	}
}

// pushLoop fires send() at the next local occurrence of hhmm, daily. A push
// that fails (server down) is retried 3× at 30s spacing, then skipped —
// never replayed later.
func pushLoop(api *apiclient.Client, tg *telegram.Client, chatID int64, hhmm string,
	send func(*apiclient.Client, *telegram.Client, int64) error, name string) {
	for {
		fire := nextFire(time.Now(), hhmm)
		log.Printf("next %s at %s", name, fire.Format("2006-01-02 15:04"))
		time.Sleep(time.Until(fire))
		var err error
		for attempt := 0; attempt < 3; attempt++ {
			if err = send(api, tg, chatID); err == nil {
				break
			}
			time.Sleep(30 * time.Second)
		}
		if err != nil {
			log.Printf("%s skipped: %v", name, err)
		}
	}
}

// sendBriefing pushes the morning briefing.
func sendBriefing(api *apiclient.Client, tg *telegram.Client, chatID int64) error {
	d, err := api.Dashboard()
	if err != nil {
		return err
	}
	return tg.SendMessage(chatID, formatBriefing(d))
}

// sendNudge pushes the evening nudge — only when nudgeQuest says so.
func sendNudge(api *apiclient.Client, tg *telegram.Client, chatID int64) error {
	d, err := api.Dashboard()
	if err != nil {
		return err
	}
	q, ok := nudgeQuest(d)
	if !ok {
		return nil
	}
	msg := fmt.Sprintf("🌙 Nothing logged today. Smallest step:\n%s\n\n/done %d and the streak lives.", questLine(*q), q.ID)
	return tg.SendMessage(chatID, msg)
}
