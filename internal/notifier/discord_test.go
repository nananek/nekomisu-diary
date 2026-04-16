package notifier

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewDiscord_EmptyURL(t *testing.T) {
	d := NewDiscord("", "https://example.com")
	if d != nil {
		t.Fatalf("expected nil when webhook URL is empty, got %v", d)
	}
}

func TestNotifyPost_NilReceiver(t *testing.T) {
	var d *Discord
	// Should not panic
	d.NotifyPost(1, "title")
}

func TestNotifyComment_NilReceiver(t *testing.T) {
	var d *Discord
	d.NotifyComment(1, 2, "post", "author", "body")
}

func captureWebhook(t *testing.T) (string, chan map[string]any, func()) {
	t.Helper()
	ch := make(chan map[string]any, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		json.Unmarshal(body, &payload)
		ch <- payload
		w.WriteHeader(http.StatusNoContent)
	}))
	return srv.URL, ch, srv.Close
}

func TestNotifyPost_SendsExpectedContent(t *testing.T) {
	url, ch, cleanup := captureWebhook(t)
	defer cleanup()

	d := NewDiscord(url, "https://diary.example.com")
	d.NotifyPost(42, "テストタイトル")

	select {
	case p := <-ch:
		content, _ := p["content"].(string)
		if !strings.Contains(content, "新しい記事が公開されました") {
			t.Errorf("missing publish phrase: %q", content)
		}
		if !strings.Contains(content, "テストタイトル") {
			t.Errorf("missing title: %q", content)
		}
		if !strings.Contains(content, "https://diary.example.com/posts/42") {
			t.Errorf("missing URL: %q", content)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("webhook never fired")
	}
}

func TestNotifyComment_SendsExpectedContent(t *testing.T) {
	url, ch, cleanup := captureWebhook(t)
	defer cleanup()

	d := NewDiscord(url, "https://diary.example.com")
	d.NotifyComment(7, 100, "記事タイトル", "猫山", "こんにちは")

	select {
	case p := <-ch:
		content, _ := p["content"].(string)
		for _, want := range []string{"💬", "猫山", "こんにちは", "記事タイトル", "/posts/7#comment-100"} {
			if !strings.Contains(content, want) {
				t.Errorf("missing %q in content: %q", want, content)
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("webhook never fired")
	}
}

func TestSend_Failure_DoesNotPanic(t *testing.T) {
	// Unreachable URL → should log and return without panic
	d := NewDiscord("http://127.0.0.1:1/unreachable", "https://example.com")
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		d.send("hello")
	}()
	wg.Wait()
}
