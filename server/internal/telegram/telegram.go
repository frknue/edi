// Package telegram is a minimal Telegram Bot API client — exactly what the
// edi presence bot needs (long-poll getUpdates, sendMessage with inline
// buttons, callback answers, message edits) and nothing more. Plain
// net/http; the Bot API is HTTPS + JSON, no SDK required.
package telegram

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Client talks to the Telegram Bot API for one bot token.
type Client struct {
	BaseURL string // default https://api.telegram.org; overridable in tests
	Token   string
	HTTP    *http.Client
}

// New returns a client. The HTTP timeout leaves headroom over the longest
// long-poll timeout we request (GetUpdates timeoutSec).
func New(token string) *Client {
	return &Client{
		BaseURL: "https://api.telegram.org",
		Token:   token,
		HTTP:    &http.Client{Timeout: 70 * time.Second},
	}
}

// Chat identifies where a message lives.
type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"` // "private" | "group" | "supergroup" | "channel"
}

// UpdateMessage is the (only) part of an incoming message the bot reads.
type UpdateMessage struct {
	MessageID int64  `json:"message_id"`
	Text      string `json:"text"`
	Chat      Chat   `json:"chat"`
}

// CallbackQuery is an inline-button press. Data is the button's payload
// (what the bot put there); Message is the message carrying the keyboard.
type CallbackQuery struct {
	ID      string         `json:"id"`
	Data    string         `json:"data"`
	Message *UpdateMessage `json:"message"`
}

// Update is one entry from getUpdates: a message, a button press, or
// something the bot ignores (both nil).
type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *UpdateMessage `json:"message"`
	CallbackQuery *CallbackQuery `json:"callback_query"`
}

// Button is one inline keyboard button; Data comes back as CallbackQuery.Data.
type Button struct {
	Text string `json:"text"`
	Data string `json:"callback_data"`
}

func markup(rows [][]Button) string {
	if len(rows) == 0 {
		return ""
	}
	b, _ := json.Marshal(map[string]any{"inline_keyboard": rows})
	return string(b)
}

type apiResponse struct {
	OK          bool            `json:"ok"`
	Description string          `json:"description"`
	Result      json.RawMessage `json:"result"`
}

func (c *Client) call(method string, params url.Values, out any) error {
	resp, err := c.HTTP.PostForm(fmt.Sprintf("%s/bot%s/%s", c.BaseURL, c.Token, method), params)
	if err != nil {
		return fmt.Errorf("telegram %s: %w", method, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("telegram %s: read: %w", method, err)
	}
	var api apiResponse
	if err := json.Unmarshal(body, &api); err != nil {
		return fmt.Errorf("telegram %s: decode: %w", method, err)
	}
	if !api.OK {
		return fmt.Errorf("telegram %s: %s", method, api.Description)
	}
	if out != nil {
		return json.Unmarshal(api.Result, out)
	}
	return nil
}

// BotInfo is the bot's own identity (from getMe).
type BotInfo struct {
	Username string `json:"username"`
}

// GetMe returns the bot's identity — used for the t.me pairing deep link and
// as a startup token check.
func (c *Client) GetMe() (BotInfo, error) {
	var me BotInfo
	err := c.call("getMe", url.Values{}, &me)
	return me, err
}

// GetUpdates long-polls for updates with update_id >= offset.
func (c *Client) GetUpdates(offset int64, timeoutSec int) ([]Update, error) {
	params := url.Values{
		"offset":          {strconv.FormatInt(offset, 10)},
		"timeout":         {strconv.Itoa(timeoutSec)},
		"allowed_updates": {`["message","callback_query"]`},
	}
	var updates []Update
	err := c.call("getUpdates", params, &updates)
	return updates, err
}

// SendMessage sends HTML-formatted text to a chat. Callers must escape
// user-derived text with html.EscapeString before embedding it.
func (c *Client) SendMessage(chatID int64, html string) error {
	params := url.Values{
		"chat_id":    {strconv.FormatInt(chatID, 10)},
		"text":       {html},
		"parse_mode": {"HTML"},
	}
	return c.call("sendMessage", params, nil)
}

// SendMessageWithButtons sends HTML text with an inline keyboard (rows of
// buttons). An empty rows slice sends a plain message.
func (c *Client) SendMessageWithButtons(chatID int64, html string, rows [][]Button) error {
	params := url.Values{
		"chat_id":    {strconv.FormatInt(chatID, 10)},
		"text":       {html},
		"parse_mode": {"HTML"},
	}
	if m := markup(rows); m != "" {
		params.Set("reply_markup", m)
	}
	return c.call("sendMessage", params, nil)
}

// EditMessageText rewrites a message the bot sent (text + keyboard). Passing
// no rows removes the buttons — how a pressed nudge becomes a receipt.
func (c *Client) EditMessageText(chatID, messageID int64, html string, rows [][]Button) error {
	params := url.Values{
		"chat_id":    {strconv.FormatInt(chatID, 10)},
		"message_id": {strconv.FormatInt(messageID, 10)},
		"text":       {html},
		"parse_mode": {"HTML"},
	}
	if m := markup(rows); m != "" {
		params.Set("reply_markup", m)
	}
	return c.call("editMessageText", params, nil)
}

// AnswerCallbackQuery acknowledges a button press (stops the client's
// spinner) with an optional short toast.
func (c *Client) AnswerCallbackQuery(id, text string) error {
	params := url.Values{"callback_query_id": {id}}
	if text != "" {
		params.Set("text", text)
	}
	return c.call("answerCallbackQuery", params, nil)
}

// SendTyping shows the "typing…" indicator in a chat for ~5s (best effort —
// used while a slow reply is being prepared).
func (c *Client) SendTyping(chatID int64) error {
	params := url.Values{
		"chat_id": {strconv.FormatInt(chatID, 10)},
		"action":  {"typing"},
	}
	return c.call("sendChatAction", params, nil)
}
