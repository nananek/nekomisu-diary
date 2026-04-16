package sanitize

import (
	"strings"
	"testing"
)

func TestHTML_RemovesScript(t *testing.T) {
	in := `<p>hi</p><script>alert(1)</script>`
	out := HTML(in)
	if strings.Contains(out, "<script") {
		t.Errorf("script not removed: %q", out)
	}
	if !strings.Contains(out, "<p>hi</p>") {
		t.Errorf("p tag lost: %q", out)
	}
}

func TestHTML_RemovesEventHandler(t *testing.T) {
	in := `<img src="x" onerror="alert(1)">`
	out := HTML(in)
	if strings.Contains(out, "onerror") {
		t.Errorf("onerror survived: %q", out)
	}
}

func TestHTML_RemovesJavascriptURL(t *testing.T) {
	in := `<a href="javascript:alert(1)">click</a>`
	out := HTML(in)
	if strings.Contains(out, "javascript:") {
		t.Errorf("javascript: URL survived: %q", out)
	}
}

func TestHTML_AllowsBasicFormatting(t *testing.T) {
	in := `<p>hello <strong>world</strong> <em>!</em></p>`
	out := HTML(in)
	for _, want := range []string{"<p>", "<strong>", "<em>"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in output: %q", want, out)
		}
	}
}

func TestHTML_AllowsFigureAndImg(t *testing.T) {
	in := `<figure class="wp-block-image"><img src="/uploads/foo.jpg" alt="x" class="size-large"/></figure>`
	out := HTML(in)
	if !strings.Contains(out, "<figure") || !strings.Contains(out, "<img") {
		t.Errorf("figure/img lost: %q", out)
	}
	if !strings.Contains(out, `class="wp-block-image"`) {
		t.Errorf("figure class stripped: %q", out)
	}
}

func TestHTML_RemovesIframe(t *testing.T) {
	in := `<iframe src="https://evil.example.com"></iframe>`
	out := HTML(in)
	if strings.Contains(out, "iframe") {
		t.Errorf("iframe survived: %q", out)
	}
}

func TestHTML_RemovesInlineStyle(t *testing.T) {
	in := `<p style="color:red">hi</p>`
	out := HTML(in)
	if strings.Contains(out, "style=") {
		t.Errorf("inline style survived: %q", out)
	}
}
