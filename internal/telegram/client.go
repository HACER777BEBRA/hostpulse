package telegram

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	token      string
	chatID     int64
	httpClient *http.Client
}

type Update struct {
	UpdateID int `json:"update_id"`
	Message  *struct {
		MessageID int `json:"message_id"`
		Chat      struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		Text string `json:"text"`
		From *struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
		} `json:"from"`
	} `json:"message"`
}

type apiResp struct {
	OK     bool     `json:"ok"`
	Result []Update `json:"result"`
	Desc   string   `json:"description"`
}

func New(token string, chatID int64) *Client {
	return &Client{
		token:  token,
		chatID: chatID,
		httpClient: &http.Client{
			Timeout: 40 * time.Second,
		},
	}
}

func (c *Client) AllowedChat(id int64) bool {
	return id == c.chatID
}

func (c *Client) Send(text string) error {
	form := url.Values{}
	form.Set("chat_id", strconv.FormatInt(c.chatID, 10))
	form.Set("text", text)
	form.Set("parse_mode", "HTML")
	form.Set("disable_web_page_preview", "true")
	_, err := c.post("sendMessage", form)
	return c.wrapErr(err)
}

func (c *Client) GetUpdates(offset int) ([]Update, error) {
	q := url.Values{}
	q.Set("timeout", "25")
	q.Set("allowed_updates", `["message"]`)
	if offset > 0 {
		q.Set("offset", strconv.Itoa(offset))
	}
	var resp apiResp
	if err := c.get("getUpdates", q, &resp); err != nil {
		return nil, fmt.Errorf("telegram getUpdates: %s", c.redact(err.Error()))
	}
	if !resp.OK {
		return nil, fmt.Errorf("telegram getUpdates: %s", resp.Desc)
	}
	return resp.Result, nil
}

func (c *Client) get(method string, q url.Values, dest *apiResp) error {
	u := fmt.Sprintf("https://api.telegram.org/bot%s/%s?%s", c.token, method, q.Encode())
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return err
	}
	return nil
}

func (c *Client) post(method string, form url.Values) ([]byte, error) {
	u := fmt.Sprintf("https://api.telegram.org/bot%s/%s", c.token, method)
	req, err := http.NewRequest(http.MethodPost, u, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		OK   bool   `json:"ok"`
		Desc string `json:"description"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return body, err
	}
	if !parsed.OK {
		return body, fmt.Errorf("%s", parsed.Desc)
	}
	return body, nil
}

func (c *Client) wrapErr(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s", c.redact(err.Error()))
}

func (c *Client) redact(s string) string {
	if c.token == "" {
		return s
	}
	return strings.ReplaceAll(s, c.token, "***")
}
