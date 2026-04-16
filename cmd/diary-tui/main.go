package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- Styles ---

var (
	titleStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#a88be0")).Bold(true)
	authorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#7c5cbf")).Bold(true)
	metaStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true)
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#d94f5c"))
	successStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#2d9d5e"))
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#a88be0")).Bold(true).Background(lipgloss.Color("236"))
	headerStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#a88be0")).Bold(true).Padding(0, 1).Border(lipgloss.NormalBorder(), false, false, true, false).BorderForeground(lipgloss.Color("240"))
)

// --- Screen types ---

type screen int

const (
	screenLogin screen = iota
	screen2FA
	screenTimeline
	screenPostDetail
	screenCompose
	screenComment
)

// --- Main model ---

type model struct {
	client    *Client
	screen    screen
	width     int
	height    int
	err       string
	me        *Me

	// Login
	loginForm loginModel
	// 2FA
	totpInput textinput.Model
	// Timeline
	timeline timelineModel
	// Post detail
	detail detailModel
	// Compose post
	compose composeModel
	// Comment
	comment commentModel
}

func (m model) Init() tea.Cmd {
	if m.client != nil {
		return loadMeCmd(m.client)
	}
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.timeline.width, m.timeline.height = msg.Width, msg.Height-4
		m.detail.vp.Width = msg.Width
		m.detail.vp.Height = msg.Height - 4
		m.compose.ta.SetWidth(msg.Width - 4)
		m.comment.ta.SetWidth(msg.Width - 4)
		return m, nil

	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}

	case errMsg:
		m.err = string(msg)
		return m, nil

	case meMsg:
		m.me = (*Me)(msg)
		if m.screen != screenTimeline {
			m.screen = screenTimeline
			return m, loadTimelineCmd(m.client)
		}
		return m, nil

	case authOKMsg:
		m.client.SaveSession()
		return m, loadMeCmd(m.client)

	case needs2FAMsg:
		m.screen = screen2FA
		ti := textinput.New()
		ti.Placeholder = "6桁コード"
		ti.CharLimit = 6
		ti.Focus()
		m.totpInput = ti
		return m, textinput.Blink

	case timelineLoadedMsg:
		m.timeline.posts = msg.posts
		m.timeline.cursor = 0
		m.client.MarkSeen()
		return m, nil

	case detailLoadedMsg:
		m.detail.post = msg.post
		m.detail.comments = msg.comments
		m.detail.renderBody(m.width)
		m.screen = screenPostDetail
		return m, nil

	case postCreatedMsg:
		m.screen = screenTimeline
		return m, loadTimelineCmd(m.client)

	case commentCreatedMsg:
		m.screen = screenPostDetail
		if m.detail.post != nil {
			return m, loadDetailCmd(m.client, m.detail.post.ID)
		}
		return m, nil
	}

	// Dispatch to the active screen
	var cmd tea.Cmd
	switch m.screen {
	case screenLogin:
		m.loginForm, cmd = m.loginForm.Update(msg)
		if m.loginForm.submit && m.client == nil {
			c, err := NewClient(m.loginForm.url.Value())
			if err != nil {
				m.err = err.Error()
				m.loginForm.submit = false
				return m, nil
			}
			m.client = c
		}
		if m.loginForm.submit {
			cmd2 := loginCmd(m.client, m.loginForm.login.Value(), m.loginForm.password.Value())
			m.loginForm.submit = false
			return m, cmd2
		}
		return m, cmd

	case screen2FA:
		if k, ok := msg.(tea.KeyMsg); ok && k.Type == tea.KeyEnter {
			return m, totpCmd(m.client, m.totpInput.Value())
		}
		m.totpInput, cmd = m.totpInput.Update(msg)
		return m, cmd

	case screenTimeline:
		return m.updateTimeline(msg)

	case screenPostDetail:
		return m.updateDetail(msg)

	case screenCompose:
		m.compose, cmd = m.compose.Update(msg, m.client)
		if m.compose.cancelled {
			m.compose.cancelled = false
			m.screen = screenTimeline
		}
		return m, cmd

	case screenComment:
		m.comment, cmd = m.comment.Update(msg, m.client)
		if m.comment.cancelled {
			m.comment.cancelled = false
			m.screen = screenPostDetail
		}
		return m, cmd
	}
	return m, nil
}

func (m model) updateTimeline(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch k.String() {
	case "q":
		return m, tea.Quit
	case "r":
		return m, loadTimelineCmd(m.client)
	case "n":
		m.compose = newComposeModel(m.width, m.height)
		m.screen = screenCompose
		return m, textarea.Blink
	case "j", "down":
		if m.timeline.cursor < len(m.timeline.posts)-1 {
			m.timeline.cursor++
		}
	case "k", "up":
		if m.timeline.cursor > 0 {
			m.timeline.cursor--
		}
	case "g":
		m.timeline.cursor = 0
	case "G":
		m.timeline.cursor = len(m.timeline.posts) - 1
	case "enter":
		if len(m.timeline.posts) > 0 {
			p := m.timeline.posts[m.timeline.cursor]
			return m, loadDetailCmd(m.client, p.ID)
		}
	}
	return m, nil
}

func (m model) updateDetail(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		m.detail.vp, cmd = m.detail.vp.Update(msg)
		return m, cmd
	}
	switch k.String() {
	case "q", "esc":
		m.screen = screenTimeline
		return m, nil
	case "c":
		m.comment = newCommentModel(m.detail.post.ID, m.width, m.height)
		m.screen = screenComment
		return m, textarea.Blink
	case "j", "down":
		m.detail.vp.LineDown(1)
	case "k", "up":
		m.detail.vp.LineUp(1)
	case "d", "pgdown":
		m.detail.vp.HalfViewDown()
	case "u", "pgup":
		m.detail.vp.HalfViewUp()
	}
	return m, nil
}

func (m model) View() string {
	switch m.screen {
	case screenLogin:
		return m.loginForm.View(m.width, m.err)
	case screen2FA:
		return m.view2FA()
	case screenTimeline:
		return m.viewTimeline()
	case screenPostDetail:
		return m.viewDetail()
	case screenCompose:
		return m.compose.View()
	case screenComment:
		return m.comment.View()
	}
	return ""
}

func (m model) view2FA() string {
	help := helpStyle.Render("Enter: 確認  /  Ctrl+C: 終了")
	return fmt.Sprintf("\n%s\n\n  %s\n\n%s\n",
		headerStyle.Render("二段階認証"),
		m.totpInput.View(),
		help,
	)
}

func (m model) viewTimeline() string {
	if len(m.timeline.posts) == 0 {
		return headerStyle.Render("タイムライン") + "\n\n  (まだ投稿がありません)\n\n" + helpStyle.Render("n: 新規  r: 更新  q: 終了")
	}

	var b strings.Builder
	name := ""
	if m.me != nil {
		name = " @" + m.me.Login
	}
	b.WriteString(headerStyle.Render("タイムライン"+name) + "\n\n")

	for i, p := range m.timeline.posts {
		line := fmt.Sprintf("  %s  %s  %s",
			authorStyle.Render(p.AuthorName),
			titleStyle.Render(p.Title),
			metaStyle.Render(fmt.Sprintf("💬 %d", p.CommentCount)),
		)
		if i == m.timeline.cursor {
			line = selectedStyle.Render("▶ " + strings.TrimLeft(line, " "))
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n" + helpStyle.Render("j/k: 移動  Enter: 開く  n: 新規  r: 更新  q: 終了"))
	if m.err != "" {
		b.WriteString("\n" + errorStyle.Render(m.err))
	}
	return b.String()
}

func (m model) viewDetail() string {
	if m.detail.post == nil {
		return "読み込み中..."
	}
	p := m.detail.post
	var b strings.Builder
	date := ""
	if p.PublishedAt != nil {
		date = *p.PublishedAt
	}
	b.WriteString(headerStyle.Render(p.Title) + "\n")
	b.WriteString(authorStyle.Render(p.AuthorName) + "  " + metaStyle.Render(date) + "\n\n")
	b.WriteString(m.detail.vp.View())
	b.WriteString("\n" + helpStyle.Render("j/k: スクロール  c: コメント  q/Esc: 戻る"))
	return b.String()
}

// --- Cmds / messages ---

type errMsg string
type meMsg *Me
type authOKMsg struct{}
type needs2FAMsg struct{}
type timelineLoadedMsg struct{ posts []Post }
type detailLoadedMsg struct {
	post     *Post
	comments []Comment
}
type postCreatedMsg struct{ id int64 }
type commentCreatedMsg struct{}

func loginCmd(c *Client, login, password string) tea.Cmd {
	return func() tea.Msg {
		r, err := c.Login(login, password)
		if err != nil {
			return errMsg(err.Error())
		}
		if r.Requires2FA {
			return needs2FAMsg{}
		}
		return authOKMsg{}
	}
}

func totpCmd(c *Client, code string) tea.Cmd {
	return func() tea.Msg {
		if err := c.VerifyTOTP(code); err != nil {
			return errMsg(err.Error())
		}
		return authOKMsg{}
	}
}

func loadMeCmd(c *Client) tea.Cmd {
	return func() tea.Msg {
		me, err := c.Me()
		if err != nil {
			return errMsg("セッションが無効です。再ログインしてください")
		}
		return meMsg(me)
	}
}

func loadTimelineCmd(c *Client) tea.Cmd {
	return func() tea.Msg {
		pl, err := c.ListPosts(1)
		if err != nil {
			return errMsg(err.Error())
		}
		return timelineLoadedMsg{posts: pl.Posts}
	}
}

func loadDetailCmd(c *Client, id int64) tea.Cmd {
	return func() tea.Msg {
		p, err := c.GetPost(id)
		if err != nil {
			return errMsg(err.Error())
		}
		cs, _ := c.ListComments(id)
		return detailLoadedMsg{post: p, comments: cs}
	}
}

func createPostCmd(c *Client, title, body string) tea.Cmd {
	return func() tea.Msg {
		id, err := c.CreatePost(title, body, body, "public")
		if err != nil {
			return errMsg(err.Error())
		}
		return postCreatedMsg{id: id}
	}
}

func createCommentCmd(c *Client, postID int64, body string) tea.Cmd {
	return func() tea.Msg {
		if err := c.CreateComment(postID, body); err != nil {
			return errMsg(err.Error())
		}
		return commentCreatedMsg{}
	}
}

// --- Sub-models ---

type loginModel struct {
	url      textinput.Model
	login    textinput.Model
	password textinput.Model
	focus    int // 0=url 1=login 2=password
	submit   bool
}

func newLoginModel() loginModel {
	urlInput := textinput.New()
	urlInput.Placeholder = "https://wordpress.tail2c8c7.ts.net"
	urlInput.SetValue("https://wordpress.tail2c8c7.ts.net")
	urlInput.Focus()

	li := textinput.New()
	li.Placeholder = "ログインID"

	pi := textinput.New()
	pi.Placeholder = "パスワード"
	pi.EchoMode = textinput.EchoPassword

	return loginModel{url: urlInput, login: li, password: pi, focus: 0}
}

func (l loginModel) Update(msg tea.Msg) (loginModel, tea.Cmd) {
	k, isKey := msg.(tea.KeyMsg)
	if isKey {
		switch k.Type {
		case tea.KeyTab, tea.KeyDown:
			l.focus = (l.focus + 1) % 3
			l.updateFocus()
			return l, nil
		case tea.KeyShiftTab, tea.KeyUp:
			l.focus = (l.focus + 2) % 3
			l.updateFocus()
			return l, nil
		case tea.KeyEnter:
			if l.focus < 2 {
				l.focus++
				l.updateFocus()
				return l, nil
			}
			if l.login.Value() != "" && l.password.Value() != "" {
				l.submit = true
				return l, nil
			}
		}
	}

	var cmd tea.Cmd
	switch l.focus {
	case 0:
		l.url, cmd = l.url.Update(msg)
	case 1:
		l.login, cmd = l.login.Update(msg)
	case 2:
		l.password, cmd = l.password.Update(msg)
	}
	return l, cmd
}

func (l *loginModel) updateFocus() {
	l.url.Blur(); l.login.Blur(); l.password.Blur()
	switch l.focus {
	case 0:
		l.url.Focus()
	case 1:
		l.login.Focus()
	case 2:
		l.password.Focus()
	}
}

func (l loginModel) View(width int, errMsg string) string {
	var b strings.Builder
	b.WriteString("\n" + headerStyle.Render("ねこのみすきー交換日記 — ログイン") + "\n\n")
	b.WriteString("  URL:      " + l.url.View() + "\n")
	b.WriteString("  ログインID: " + l.login.View() + "\n")
	b.WriteString("  パスワード: " + l.password.View() + "\n\n")
	if errMsg != "" {
		b.WriteString(errorStyle.Render("  ✗ "+errMsg) + "\n\n")
	}
	b.WriteString(helpStyle.Render("  Tab: 次へ  /  Enter: 決定  /  Ctrl+C: 終了"))
	return b.String()
}

type timelineModel struct {
	posts  []Post
	cursor int
	width  int
	height int
}

type detailModel struct {
	post     *Post
	comments []Comment
	vp       viewport.Model
}

func (d *detailModel) renderBody(width int) {
	if d.post == nil {
		return
	}
	// Simple HTML-to-text stripper; not perfect but keeps it readable.
	body := stripTags(d.post.BodyHTML)
	if d.post.BodyMD != nil {
		body = *d.post.BodyMD
	}
	var b strings.Builder
	b.WriteString(body)
	b.WriteString("\n\n" + metaStyle.Render(fmt.Sprintf("── コメント (%d) ──", len(d.comments))) + "\n\n")
	for _, c := range d.comments {
		indent := ""
		if c.ParentID != nil {
			indent = "    "
		}
		b.WriteString(fmt.Sprintf("%s%s  %s\n", indent, authorStyle.Render(c.AuthorName), metaStyle.Render(c.CreatedAt[:10])))
		b.WriteString(indent + c.Body + "\n\n")
	}
	d.vp.SetContent(b.String())
}

func stripTags(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

type composeModel struct {
	title     textinput.Model
	ta        textarea.Model
	focus     int // 0=title 1=body
	cancelled bool
	submitted bool
}

func newComposeModel(w, h int) composeModel {
	ti := textinput.New()
	ti.Placeholder = "タイトル"
	ti.Focus()
	ta := textarea.New()
	ta.Placeholder = "本文 (Markdown) — Ctrl+D で投稿、Esc でキャンセル"
	ta.SetWidth(w - 4)
	ta.SetHeight(h - 8)
	return composeModel{title: ti, ta: ta, focus: 0}
}

func (c composeModel) Update(msg tea.Msg, cl *Client) (composeModel, tea.Cmd) {
	k, isKey := msg.(tea.KeyMsg)
	if isKey {
		switch k.Type {
		case tea.KeyEsc:
			c.cancelled = true
			return c, nil
		case tea.KeyCtrlD:
			if c.title.Value() != "" && strings.TrimSpace(c.ta.Value()) != "" {
				return c, createPostCmd(cl, c.title.Value(), c.ta.Value())
			}
		case tea.KeyTab:
			c.focus = 1 - c.focus
			if c.focus == 0 {
				c.title.Focus()
				c.ta.Blur()
			} else {
				c.title.Blur()
				c.ta.Focus()
			}
			return c, nil
		}
	}
	var cmd tea.Cmd
	if c.focus == 0 {
		c.title, cmd = c.title.Update(msg)
	} else {
		c.ta, cmd = c.ta.Update(msg)
	}
	return c, cmd
}

func (c composeModel) View() string {
	var b strings.Builder
	b.WriteString("\n" + headerStyle.Render("新しい日記") + "\n\n")
	b.WriteString("  " + c.title.View() + "\n\n")
	b.WriteString(c.ta.View())
	b.WriteString("\n\n" + helpStyle.Render("Tab: 切替  Ctrl+D: 投稿  Esc: キャンセル"))
	return b.String()
}

type commentModel struct {
	postID    int64
	ta        textarea.Model
	cancelled bool
}

func newCommentModel(postID int64, w, h int) commentModel {
	ta := textarea.New()
	ta.Placeholder = "コメント — Ctrl+D で送信、Esc でキャンセル"
	ta.SetWidth(w - 4)
	ta.SetHeight(8)
	ta.Focus()
	return commentModel{postID: postID, ta: ta}
}

func (c commentModel) Update(msg tea.Msg, cl *Client) (commentModel, tea.Cmd) {
	k, isKey := msg.(tea.KeyMsg)
	if isKey {
		switch k.Type {
		case tea.KeyEsc:
			c.cancelled = true
			return c, nil
		case tea.KeyCtrlD:
			if strings.TrimSpace(c.ta.Value()) != "" {
				return c, createCommentCmd(cl, c.postID, c.ta.Value())
			}
		}
	}
	var cmd tea.Cmd
	c.ta, cmd = c.ta.Update(msg)
	return c, cmd
}

func (c commentModel) View() string {
	var b strings.Builder
	b.WriteString("\n" + headerStyle.Render("コメントする") + "\n\n")
	b.WriteString(c.ta.View())
	b.WriteString("\n\n" + helpStyle.Render("Ctrl+D: 送信  Esc: キャンセル"))
	return b.String()
}

// --- Entry point ---

func main() {
	// Try to load existing session
	client, err := LoadClient()
	var m model
	if err == nil && client != nil {
		// Verify session still valid
		if _, err := client.Me(); err == nil {
			m = model{client: client, screen: screenTimeline}
			m.detail.vp = viewport.New(80, 20)
			p := tea.NewProgram(m, tea.WithAltScreen())
			if _, err := p.Run(); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		}
	}

	// Fresh login
	m = model{
		screen:    screenLogin,
		loginForm: newLoginModel(),
	}
	m.detail.vp = viewport.New(80, 20)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
