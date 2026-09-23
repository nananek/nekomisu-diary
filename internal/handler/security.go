package handler

import "net/http"

// securityPolicy is the app-wide Content Security Policy.
//
// scripts: only our own bundle. 'unsafe-inline'/eval/handlers stay blocked,
// so even a sanitizer bypass cannot execute injected script or exfiltrate
// via fetch/XHR (connect-src 'self'). External HTTPS images remain allowed
// because migrated posts and Markdown can hotlink them; Referrer-Policy
// keeps page URLs out of those requests.
//
// styles: stylesheets from our origin and inline style *attributes*
// (React renders, Markdown preview). <style> elements stay blocked.
const securityPolicy = "default-src 'self'; " +
	"script-src 'self'; " +
	"script-src-attr 'none'; " +
	"style-src 'self'; " +
	"style-src-attr 'unsafe-inline'; " +
	"img-src 'self' data: https:; " +
	"connect-src 'self'; " +
	"font-src 'self'; " +
	"object-src 'none'; " +
	"base-uri 'none'; " +
	"form-action 'self'; " +
	"frame-src 'none'; " +
	"frame-ancestors 'none'; " +
	"worker-src 'self'; " +
	"manifest-src 'self'"

// SecurityHeaders applies baseline browser hardening to every response.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", securityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}
