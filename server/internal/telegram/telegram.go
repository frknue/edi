// Package telegram is a minimal Telegram Bot API client — exactly what the
// edi presence bot needs (long-poll getUpdates + sendMessage) and nothing
// more. Plain net/http; the Bot API is HTTPS + JSON, no SDK required.
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

// UpdateMessage is the (only) part of an incoming message the bot reads.
type UpdateMessage struct {
	Text string `json:"text"`
	Chat struct {
		ID int64 `json:"id"`
	} `json:"chat"`
}

// Update is one entry from getUpdates. Message is nil for non-message
// updates (edits, joins, …), which the bot ignores.
type Update struct {
	UpdateID int64          `json:"update_id"`
	Message  *UpdateMessage `json:"message"`
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

// GetUpdates long-polls for updates with update_id >= offset.
func (c *Client) GetUpdates(offset int64, timeoutSec int) ([]Update, error) {
	params := url.Values{
		"offset":          {strconv.FormatInt(offset, 10)},
		"timeout":         {strconv.Itoa(timeoutSec)},
		"allowed_updates": {`["message"]`},
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
