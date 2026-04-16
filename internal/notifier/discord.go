package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

type Discord struct {
	webhookURL string
	baseURL    string
	client     *http.Client
}

func NewDiscord(webhookURL, baseURL string) *Discord {
	if webhookURL == "" {
		return nil
	}
	return &Discord{
		webhookURL: webhookURL,
		baseURL:    baseURL,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

func (d *Discord) NotifyPost(postID int64, title string) {
	if d == nil {
		return
	}
	go d.send(fmt.Sprintf(
		"📰 新しい記事が公開されました！\n**%s**\n%s/posts/%d",
		title, d.baseURL, postID,
	))
}

func (d *Discord) NotifyComment(postID, commentID int64, postTitle, author, body string) {
	if d == nil {
		return
	}
	go d.send(fmt.Sprintf(
		"💬 新しいコメントが投稿されました：\n**%s** さんより\n「%s」\n投稿：**%s**\n%s/posts/%d#comment-%d",
		author, body, postTitle, d.baseURL, postID, commentID,
	))
}

func (d *Discord) send(content string) {
	payload, _ := json.Marshal(map[string]string{"content": content})
	req, err := http.NewRequest("POST", d.webhookURL, bytes.NewReader(payload))
	if err != nil {
		log.Printf("discord: build req: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := d.client.Do(req)
	if err != nil {
		log.Printf("discord: send: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		log.Printf("discord: status %d", resp.StatusCode)
	}
}
