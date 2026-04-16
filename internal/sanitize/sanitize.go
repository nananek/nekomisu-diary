// Package sanitize provides server-side HTML sanitization for user posts.
// Applied on write so the DB always stores safe HTML (no scripts, no event
// handlers, no data URIs in src, etc.).
package sanitize

import (
	"sync"

	"github.com/microcosm-cc/bluemonday"
)

var (
	once   sync.Once
	policy *bluemonday.Policy
)

// Policy returns the shared bluemonday policy used for post bodies.
// Based on UGCPolicy with a few additions suitable for diary-style content.
func Policy() *bluemonday.Policy {
	once.Do(func() {
		p := bluemonday.UGCPolicy()
		// Allow figure / figcaption (used by Gutenberg-rendered posts),
		// plus sup/sub for footnotes etc.
		p.AllowElements("figure", "figcaption", "sup", "sub")
		// Allow class on common block elements so migrated WP blocks
		// that carry wp-block-* classes survive.
		p.AllowAttrs("class").OnElements(
			"div", "figure", "figcaption", "img", "p", "span", "a",
			"pre", "code", "blockquote", "ul", "ol", "li", "table", "td", "th",
			"h1", "h2", "h3", "h4", "h5", "h6",
		)
		// Responsive-image attributes on <img> (all safe; no scripts possible).
		p.AllowAttrs("srcset", "sizes", "loading", "decoding").OnElements("img")
		// Explicitly disallow inline style / event-handlers (bluemonday default).
		policy = p
	})
	return policy
}

// HTML returns a sanitized copy of the given HTML string.
func HTML(s string) string {
	return Policy().Sanitize(s)
}
