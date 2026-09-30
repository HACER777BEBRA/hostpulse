package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
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
	transport  *http.Transport
	stacks     *stackPreference
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
	stacks := &stackPreference{}
	httpClient, transport := newHTTPClient(stacks)
	return &Client{
		token:      token,
		chatID:     chatID,
		httpClient: httpClient,
		transport:  transport,
		stacks:     stacks,
	}
}

func newHTTPClient(stacks *stackPreference) (*http.Client, *http.Transport) {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok || stacks == nil {
		return &http.Client{Timeout: 40 * time.Second}, nil
	}
	transport := base.Clone()
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return stacks.dial(ctx, dialer, addr)
	}
	return &http.Client{
		Timeout:   40 * time.Second,
		Transport: transport,
	}, transport
}

func (c *Client) AllowedChat(id int64) bool {
	return id == c.chatID
}

func (c *Client) Send(text string) error {
	return c.SendTo(c.chatID, text)
}

func (c *Client) SendTo(chatID int64, text string) error {
	_, err := c.SendMessage(chatID, text)
	return err
}

func (c *Client) SendMessage(chatID int64, text string) (int, error) {
	form := url.Values{}
	form.Set("chat_id", strconv.FormatInt(chatID, 10))
	form.Set("text", text)
	form.Set("parse_mode", "HTML")
	form.Set("disable_web_page_preview", "true")
	body, err := c.post("sendMessage", form)
	if err != nil {
		return 0, c.wrapErr(err)
	}
	var parsed struct {
		Result struct {
			MessageID int `json:"message_id"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return 0, err
	}
	if parsed.Result.MessageID == 0 {
		return 0, fmt.Errorf("telegram sendMessage: empty message_id")
	}
	return parsed.Result.MessageID, nil
}

func (c *Client) DeleteMessage(chatID int64, messageID int) error {
	form := url.Values{}
	form.Set("chat_id", strconv.FormatInt(chatID, 10))
	form.Set("message_id", strconv.Itoa(messageID))
	_, err := c.post("deleteMessage", form)
	return c.wrapErr(err)
}

func (c *Client) EditMessage(chatID int64, messageID int, text string) error {
	form := url.Values{}
	form.Set("chat_id", strconv.FormatInt(chatID, 10))
	form.Set("message_id", strconv.Itoa(messageID))
	form.Set("text", text)
	form.Set("parse_mode", "HTML")
	form.Set("disable_web_page_preview", "true")
	_, err := c.post("editMessageText", form)
	return c.wrapErr(err)
}

// IsMember reports whether userID belongs to chatID. Private commands are
// allowed for people who are still in the configured monitoring chat.
func (c *Client) IsMember(chatID, userID int64) (bool, error) {
	q := url.Values{}
	q.Set("chat_id", strconv.FormatInt(chatID, 10))
	q.Set("user_id", strconv.FormatInt(userID, 10))
	u := fmt.Sprintf("https://api.telegram.org/bot%s/getChatMember?%s", c.token, q.Encode())
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return false, err
	}
	res, err := c.do(req)
	if err != nil {
		return false, c.wrapErr(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return false, err
	}
	var parsed struct {
		OK     bool   `json:"ok"`
		Desc   string `json:"description"`
		Result struct {
			Status string `json:"status"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return false, err
	}
	if !parsed.OK {
		desc := strings.ToLower(parsed.Desc)
		if strings.Contains(desc, "user not found") || strings.Contains(desc, "not a member") || strings.Contains(desc, "user_not_participant") {
			return false, nil
		}
		return false, fmt.Errorf("%s", parsed.Desc)
	}
	return memberStatusAllowed(parsed.Result.Status), nil
}

func memberStatusAllowed(status string) bool {
	switch status {
	case "creator", "administrator", "member", "restricted":
		return true
	default:
		return false
	}
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
	res, err := c.do(req)
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
	res, err := c.do(req)
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

func (c *Client) do(req *http.Request) (*http.Response, error) {
	res, err := c.httpClient.Do(req)
	if err == nil || !isTransportFailure(err) {
		return res, err
	}
	c.noteFailure(err)
	retry, rerr := retryRequest(req)
	if rerr != nil {
		return nil, err
	}
	res, err2 := c.httpClient.Do(retry)
	if err2 != nil {
		c.noteFailure(err2)
		return nil, err2
	}
	return res, nil
}

func (c *Client) noteFailure(err error) {
	if c.stacks != nil {
		c.stacks.noteTransportErr(err)
	}
	if c.transport != nil {
		c.transport.CloseIdleConnections()
	}
}

func retryRequest(req *http.Request) (*http.Request, error) {
	clone := req.Clone(req.Context())
	if req.Body == nil || req.Body == http.NoBody {
		return clone, nil
	}
	if req.GetBody == nil {
		return nil, fmt.Errorf("cannot retry request body")
	}
	body, err := req.GetBody()
	if err != nil {
		return nil, err
	}
	clone.Body = body
	return clone, nil
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
