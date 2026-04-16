package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	jar     *cookiejar.Jar
	hc      *http.Client
}

func NewClient(baseURL string) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		jar:     jar,
		hc:      &http.Client{Jar: jar, Timeout: 30 * time.Second},
	}, nil
}

func configDir() string {
	if v := os.Getenv("DIARY_TUI_CONFIG"); v != "" {
		return v
	}
	base, _ := os.UserConfigDir()
	return filepath.Join(base, "diary-tui")
}

type persistedCookie struct {
	Name    string    `json:"name"`
	Value   string    `json:"value"`
	Domain  string    `json:"domain"`
	Path    string    `json:"path"`
	Expires time.Time `json:"expires"`
}

type persistedState struct {
	BaseURL string            `json:"base_url"`
	Cookies []persistedCookie `json:"cookies"`
}

func (c *Client) SaveSession() error {
	dir := configDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	u, _ := url.Parse(c.baseURL)
	cookies := c.jar.Cookies(u)
	state := persistedState{BaseURL: c.baseURL}
	for _, ck := range cookies {
		state.Cookies = append(state.Cookies, persistedCookie{
			Name: ck.Name, Value: ck.Value, Domain: u.Host, Path: "/",
		})
	}
	data, _ := json.MarshalIndent(state, "", "  ")
	return os.WriteFile(filepath.Join(dir, "session.json"), data, 0o600)
}

func LoadClient() (*Client, error) {
	data, err := os.ReadFile(filepath.Join(configDir(), "session.json"))
	if err != nil {
		return nil, err
	}
	var state persistedState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	c, err := NewClient(state.BaseURL)
	if err != nil {
		return nil, err
	}
	u, _ := url.Parse(c.baseURL)
	var cookies []*http.Cookie
	for _, pc := range state.Cookies {
		cookies = append(cookies, &http.Cookie{Name: pc.Name, Value: pc.Value, Path: "/"})
	}
	c.jar.SetCookies(u, cookies)
	return c, nil
}

func (c *Client) do(method, path string, body any) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.baseURL+path, r)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.hc.Do(req)
}

func (c *Client) call(method, path string, body any, out any) error {
	resp, err := c.do(method, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		var e struct{ Error string `json:"error"` }
		json.Unmarshal(data, &e)
		if e.Error != "" {
			return fmt.Errorf("%s", e.Error)
		}
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

// --- API ---

type LoginResult struct {
	OK          bool `json:"ok"`
	Requires2FA bool `json:"requires_2fa"`
	HasTOTP     bool `json:"has_totp"`
	HasWebAuthn bool `json:"has_webauthn"`
}

func (c *Client) Login(login, password string) (*LoginResult, error) {
	var out LoginResult
	err := c.call("POST", "/api/auth/login", map[string]string{"login": login, "password": password}, &out)
	return &out, err
}

func (c *Client) VerifyTOTP(code string) error {
	return c.call("POST", "/api/auth/totp/verify-login", map[string]string{"code": code}, nil)
}

type Me struct {
	ID          int64  `json:"id"`
	Login       string `json:"login"`
	DisplayName string `json:"display_name"`
	Has2FA      bool   `json:"has_2fa"`
}

func (c *Client) Me() (*Me, error) {
	var me Me
	err := c.call("GET", "/api/auth/me", nil, &me)
	return &me, err
}

type Post struct {
	ID           int64   `json:"id"`
	AuthorID     int64   `json:"author_id"`
	AuthorName   string  `json:"author_name"`
	Title        string  `json:"title"`
	BodyHTML     string  `json:"body_html"`
	BodyMD       *string `json:"body_md"`
	Visibility   string  `json:"visibility"`
	PublishedAt  *string `json:"published_at"`
	CreatedAt    string  `json:"created_at"`
	CommentCount int     `json:"comment_count"`
}

type PostList struct {
	Posts []Post `json:"posts"`
	Total int    `json:"total"`
	Page  int    `json:"page"`
	Pages int    `json:"pages"`
}

func (c *Client) ListPosts(page int) (*PostList, error) {
	var out PostList
	err := c.call("GET", fmt.Sprintf("/api/posts?page=%d", page), nil, &out)
	return &out, err
}

func (c *Client) GetPost(id int64) (*Post, error) {
	var p Post
	err := c.call("GET", fmt.Sprintf("/api/posts/%d", id), nil, &p)
	return &p, err
}

func (c *Client) CreatePost(title, bodyHTML, bodyMD, visibility string) (int64, error) {
	var out struct{ ID int64 `json:"id"` }
	body := map[string]any{"title": title, "body": bodyHTML, "visibility": visibility}
	if bodyMD != "" {
		body["body_md"] = bodyMD
	}
	err := c.call("POST", "/api/posts", body, &out)
	return out.ID, err
}

type Comment struct {
	ID         int64   `json:"id"`
	PostID     int64   `json:"post_id"`
	AuthorName string  `json:"author_name"`
	Body       string  `json:"body"`
	ParentID   *int64  `json:"parent_id"`
	CreatedAt  string  `json:"created_at"`
}

func (c *Client) ListComments(postID int64) ([]Comment, error) {
	var out struct{ Comments []Comment `json:"comments"` }
	err := c.call("GET", fmt.Sprintf("/api/posts/%d/comments", postID), nil, &out)
	return out.Comments, err
}

func (c *Client) CreateComment(postID int64, body string) error {
	return c.call("POST", fmt.Sprintf("/api/posts/%d/comments", postID), map[string]string{"body": body}, nil)
}

func (c *Client) MarkSeen() error {
	return c.call("POST", "/api/unread/mark-seen", nil, nil)
}
