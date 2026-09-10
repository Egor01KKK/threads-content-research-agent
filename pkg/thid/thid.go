// Package thid classifies the handles, URLs, shortcodes, and numeric ids that
// Threads uses, and normalizes them to a single typed Identity. It does no
// network IO, so it is safe to import anywhere and cheap to test.
package thid

import (
	"regexp"
	"strings"
)

// Kind is the category of a classified input.
type Kind string

const (
	// KindProfile is a user handle or numeric user id.
	KindProfile Kind = "profile"
	// KindPost is a post permalink, shortcode, or numeric post id.
	KindPost Kind = "post"
	// KindUnknown is anything that does not resolve to a Threads entity.
	KindUnknown Kind = "unknown"
)

// Identity is the normalized result of classifying an input.
type Identity struct {
	Kind      Kind   `json:"kind"`
	Handle    string `json:"handle,omitempty"`    // without the leading @
	Shortcode string `json:"shortcode,omitempty"` // post code, when known
	PK        string `json:"pk,omitempty"`        // numeric id, when known
	URL       string `json:"url"`                 // canonical threads.com URL
}

var (
	// handleRe matches a bare Threads handle: letters, digits, underscores,
	// and dots, optionally prefixed with @.
	handleRe = regexp.MustCompile(`^@?([A-Za-z0-9._]{1,40})$`)
	// shortcodeRe matches a bare Threads/Instagram media shortcode.
	shortcodeRe = regexp.MustCompile(`^[A-Za-z0-9_-]{6,20}$`)
	// numericRe matches a bare numeric id (pk).
	numericRe = regexp.MustCompile(`^\d{5,25}$`)
	// postPathRe pulls the handle and shortcode out of a post URL path.
	postPathRe = regexp.MustCompile(`/@?([A-Za-z0-9._]+)/post/([A-Za-z0-9_-]+)`)
	// handlePathRe pulls a handle out of a profile URL path.
	handlePathRe = regexp.MustCompile(`/@([A-Za-z0-9._]+)`)
)

// Classify resolves any input a human might paste into a typed Identity.
func Classify(input string) Identity {
	s := strings.TrimSpace(input)
	if s == "" {
		return Identity{Kind: KindUnknown}
	}

	// A URL (any threads host, or a bare threads.com/... path).
	if looksLikeURL(s) {
		return classifyURL(s)
	}

	// A bare shortcode or numeric id without a handle resolves to a post when
	// it cannot be a handle. A numeric value is a post pk by default.
	switch {
	case numericRe.MatchString(s):
		return Identity{Kind: KindPost, PK: s, URL: ""}
	case handleRe.MatchString(s):
		h := strings.TrimPrefix(s, "@")
		return Identity{Kind: KindProfile, Handle: h, URL: profileURL(h)}
	case shortcodeRe.MatchString(s):
		return Identity{Kind: KindPost, Shortcode: s, URL: ""}
	}
	return Identity{Kind: KindUnknown}
}

func looksLikeURL(s string) bool {
	return strings.Contains(s, "threads.net") ||
		strings.Contains(s, "threads.com") ||
		strings.HasPrefix(s, "http://") ||
		strings.HasPrefix(s, "https://")
}

func classifyURL(raw string) Identity {
	// Rewrite the .net host to .com up front; .net 301-redirects to .com and the
	// crawler surface lives on .com.
	u := strings.ReplaceAll(raw, "threads.net", "threads.com")

	if m := postPathRe.FindStringSubmatch(u); m != nil {
		handle := strings.TrimPrefix(m[1], "@")
		code := m[2]
		return Identity{
			Kind:      KindPost,
			Handle:    handle,
			Shortcode: code,
			URL:       postURL(handle, code),
		}
	}
	if m := handlePathRe.FindStringSubmatch(u); m != nil {
		handle := m[1]
		return Identity{Kind: KindProfile, Handle: handle, URL: profileURL(handle)}
	}
	return Identity{Kind: KindUnknown, URL: u}
}

func profileURL(handle string) string {
	return "https://www.threads.com/@" + handle
}

func postURL(handle, code string) string {
	if handle == "" {
		return ""
	}
	return "https://www.threads.com/@" + handle + "/post/" + code
}

// alphabet is the base64 ordering Instagram/Threads use to encode a numeric pk
// into a shortcode.
const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"

// ShortcodeToPK decodes a Threads shortcode into its numeric pk. It returns an
// empty string if the code contains a character outside the alphabet.
func ShortcodeToPK(code string) string {
	var n uint64
	for _, r := range code {
		idx := strings.IndexRune(alphabet, r)
		if idx < 0 {
			return ""
		}
		n = n*64 + uint64(idx)
	}
	if n == 0 {
		return ""
	}
	return uitoa(n)
}

// PKToShortcode encodes a numeric pk string into its Threads shortcode. It
// returns an empty string if pk is not a positive integer.
func PKToShortcode(pk string) string {
	n, ok := atou(pk)
	if !ok || n == 0 {
		return ""
	}
	var b []byte
	for n > 0 {
		b = append([]byte{alphabet[n%64]}, b...)
		n /= 64
	}
	return string(b)
}

func uitoa(n uint64) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func atou(s string) (uint64, bool) {
	if s == "" {
		return 0, false
	}
	var n uint64
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + uint64(r-'0')
	}
	return n, true
}
