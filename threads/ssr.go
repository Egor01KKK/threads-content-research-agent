package threads

import (
	"encoding/json"
	htmlpkg "html"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// The SSR surface: a crawler-rendered Threads page carries og: meta tags plus
// one or more <script type="application/json" data-sjs> blocks whose JSON holds
// the user object and the thread_items[].post records. These primitives decode
// that surface with the standard library alone.

var (
	ogRe     = regexp.MustCompile(`<meta[^>]+property="og:([a-z_:]+)"[^>]+content="([^"]*)"`)
	ogRe2    = regexp.MustCompile(`<meta[^>]+content="([^"]*)"[^>]+property="og:([a-z_:]+)"`)
	sjsRe    = regexp.MustCompile(`(?s)<script type="application/json"[^>]*>(.*?)</script>`)
	atHandle = regexp.MustCompile(`@([A-Za-z0-9._]+)`)
)

// ogMeta pulls every og:* value out of the page, handling both attribute
// orderings Threads emits.
func ogMeta(html string) map[string]string {
	out := map[string]string{}
	for _, m := range ogRe.FindAllStringSubmatch(html, -1) {
		out[m[1]] = htmlpkg.UnescapeString(m[2])
	}
	for _, m := range ogRe2.FindAllStringSubmatch(html, -1) {
		if _, ok := out[m[2]]; !ok {
			out[m[2]] = htmlpkg.UnescapeString(m[1])
		}
	}
	return out
}

// dataSJSBlocks returns every embedded JSON payload, HTML-unescaped.
func dataSJSBlocks(html string) []string {
	matches := sjsRe.FindAllStringSubmatch(html, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, htmlpkg.UnescapeString(m[1]))
	}
	return out
}

// parseProfileSSR builds a Profile from a crawler-rendered profile page: og tags
// for the headline fields, then the embedded user object for the exact id, bio,
// counts, and verified flag.
func parseProfileSSR(html, handle string) *Profile {
	p := &Profile{Username: handle, URL: WebBase + "/@" + handle, FetchedAt: time.Now()}

	og := ogMeta(html)
	if title := og["title"]; title != "" {
		if idx := strings.Index(title, "("); idx > 0 {
			p.Name = strings.TrimSpace(title[:idx])
		}
		if m := atHandle.FindStringSubmatch(title); m != nil {
			p.Username = m[1]
		}
	}
	if desc := og["description"]; desc != "" {
		p.Biography = desc
		if n, ok := countFromDescription(desc, "follower"); ok {
			p.FollowerCount = n
		}
	}
	if img := og["image"]; img != "" {
		p.ProfilePicURL = img
	}

	// The richer user object (follower_count + biography present) overrides the
	// og-derived fields where it exists.
	for _, raw := range dataSJSBlocks(html) {
		if !strings.Contains(raw, "follower_count") || !strings.Contains(raw, "biography") {
			continue
		}
		var data any
		if json.Unmarshal([]byte(raw), &data) != nil {
			continue
		}
		if u := findUser(data, 0); u != nil {
			mergeProfile(p, u)
			break
		}
	}
	// A real crawler-rendered profile always embeds the user object, the only
	// place the numeric id appears. Without it the page is the logged-out wall
	// Threads serves for a handle that does not resolve to a public profile, so
	// its og:description is the site's generic text, not a bio. Treat that as no
	// profile rather than returning the wall's boilerplate as data.
	if p.Username == "" || p.ID == "" {
		return nil
	}
	return p
}

// parsePostsSSR walks every embedded block that carries engagement counts and
// returns the thread_items[].post records in document order.
func parsePostsSSR(html string) []Post {
	var posts []Post
	seen := map[string]bool{}
	for _, raw := range dataSJSBlocks(html) {
		if !strings.Contains(raw, "like_count") {
			continue
		}
		var data any
		if json.Unmarshal([]byte(raw), &data) != nil {
			continue
		}
		for _, p := range walkThreadItems(data, 0) {
			if p.ID == "" || seen[p.ID] {
				continue
			}
			seen[p.ID] = true
			posts = append(posts, p)
		}
	}
	return posts
}

// parseSearchPostsSSR extracts only posts below the current search result
// payload. The generic parsePostsSSR walker is intentionally broader because
// profile and post pages use several thread_items sections; using it for
// search could accidentally include recommended or unrelated page sections.
// The bool reports whether a real searchResults object with an edges array was
// present, including a valid empty result page.
func parseSearchPostsSSR(html string) ([]Post, bool) {
	var posts []Post
	seen := map[string]bool{}
	validSearchPayload := false
	for _, raw := range dataSJSBlocks(html) {
		if !strings.Contains(raw, `"searchResults"`) ||
			!strings.Contains(raw, "BarcelonaSearchResultsQuery") {
			continue
		}
		var data any
		if json.Unmarshal([]byte(raw), &data) != nil {
			continue
		}
		for _, results := range findSearchResults(data, 0) {
			edges, ok := results["edges"].([]any)
			if !ok {
				continue
			}
			validSearchPayload = true
			for _, edge := range edges {
				em, ok := edge.(map[string]any)
				if !ok {
					continue
				}
				node, ok := em["node"].(map[string]any)
				if !ok {
					continue
				}
				thread, ok := node["thread"].(map[string]any)
				if !ok {
					continue
				}
				items, ok := thread["thread_items"].([]any)
				if !ok {
					continue
				}
				for _, item := range items {
					im, ok := item.(map[string]any)
					if !ok {
						continue
					}
					pm, ok := im["post"].(map[string]any)
					if !ok {
						continue
					}
					post := parsePost(pm)
					if post.ID == "" || seen[post.ID] {
						continue
					}
					seen[post.ID] = true
					posts = append(posts, post)
				}
			}
		}
	}
	return posts, validSearchPayload
}

// pageInfoSSR returns the end_cursor and has_next_page for GraphQL pagination.
func pageInfoSSR(html string) (cursor string, hasMore bool) {
	for _, raw := range dataSJSBlocks(html) {
		if !strings.Contains(raw, "end_cursor") {
			continue
		}
		var data any
		if json.Unmarshal([]byte(raw), &data) != nil {
			continue
		}
		if c, h, ok := findPageInfo(data, 0); ok {
			return c, h
		}
	}
	return "", false
}

// ── recursive walkers over the decoded JSON ──────────────

func walkThreadItems(data any, depth int) []Post {
	if depth > 30 {
		return nil
	}
	var out []Post
	switch v := data.(type) {
	case map[string]any:
		if items, ok := v["thread_items"].([]any); ok {
			for _, item := range items {
				im, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if pm, ok := im["post"].(map[string]any); ok {
					if p := parsePost(pm); p.ID != "" {
						out = append(out, p)
					}
				}
			}
		}
		for _, val := range v {
			out = append(out, walkThreadItems(val, depth+1)...)
		}
	case []any:
		for _, item := range v {
			out = append(out, walkThreadItems(item, depth+1)...)
		}
	}
	return out
}

func findSearchResults(data any, depth int) []map[string]any {
	if depth > 30 {
		return nil
	}
	var out []map[string]any
	switch v := data.(type) {
	case map[string]any:
		if results, ok := v["searchResults"].(map[string]any); ok {
			out = append(out, results)
		}
		for _, value := range v {
			out = append(out, findSearchResults(value, depth+1)...)
		}
	case []any:
		for _, value := range v {
			out = append(out, findSearchResults(value, depth+1)...)
		}
	}
	return out
}

func findUser(data any, depth int) map[string]any {
	if depth > 30 {
		return nil
	}
	switch v := data.(type) {
	case map[string]any:
		_, hasU := v["username"]
		_, hasF := v["follower_count"]
		_, hasB := v["biography"]
		if hasU && hasF && hasB {
			return v
		}
		for _, val := range v {
			if u := findUser(val, depth+1); u != nil {
				return u
			}
		}
	case []any:
		for _, item := range v {
			if u := findUser(item, depth+1); u != nil {
				return u
			}
		}
	}
	return nil
}

func findPageInfo(data any, depth int) (string, bool, bool) {
	if depth > 30 {
		return "", false, false
	}
	switch v := data.(type) {
	case map[string]any:
		if pi, ok := v["page_info"].(map[string]any); ok {
			cursor, _ := pi["end_cursor"].(string)
			more, _ := pi["has_next_page"].(bool)
			if cursor != "" {
				return cursor, more, true
			}
		}
		for _, val := range v {
			if c, m, ok := findPageInfo(val, depth+1); ok {
				return c, m, ok
			}
		}
	case []any:
		for _, item := range v {
			if c, m, ok := findPageInfo(item, depth+1); ok {
				return c, m, ok
			}
		}
	}
	return "", false, false
}

// ── single-record decoders ───────────────────────────────

func parsePost(data map[string]any) Post {
	p := Post{FetchedAt: time.Now()}

	p.ID = asString(data["pk"])
	if code := asString(data["code"]); code != "" {
		p.Shortcode = code
	}
	if capt, ok := data["caption"].(map[string]any); ok {
		p.Text = asString(capt["text"])
	}
	if ts := asFloat(data["taken_at"]); ts > 0 {
		p.Timestamp = time.Unix(int64(ts), 0).UTC()
	}
	p.LikeCount, p.LikeCountAvailable = metricInt(data, "like_count")
	if p.ViewCount, p.ViewCountAvailable = metricInt(data, "view_count"); !p.ViewCountAvailable {
		p.ViewCount, p.ViewCountAvailable = metricInt(data, "play_count")
	}

	switch int(asFloat(data["media_type"])) {
	case 1:
		p.MediaType, p.HasMedia = "IMAGE", true
		if u := firstImageURL(data); u != "" {
			p.MediaURLs = []string{u}
		}
	case 2:
		p.MediaType, p.HasMedia = "VIDEO", true
		if u := firstVideoURL(data); u != "" {
			p.MediaURLs = []string{u}
		}
	case 8:
		p.MediaType, p.HasMedia = "CAROUSEL_ALBUM", true
		p.MediaURLs = carouselURLs(data)
	default:
		p.MediaType = "TEXT_POST"
	}

	if user, ok := data["user"].(map[string]any); ok {
		p.Username = asString(user["username"])
		p.UserID = asString(user["pk"])
	}

	if tpi, ok := data["text_post_app_info"].(map[string]any); ok {
		p.ReplyCount, p.ReplyCountAvailable = metricInt(tpi, "direct_reply_count")
		p.RepostCount, p.RepostCountAvailable = metricInt(tpi, "repost_count")
		p.QuoteCount, p.QuoteCountAvailable = metricInt(tpi, "quote_count")
		if q, ok := tpi["is_quote_post"].(bool); ok {
			p.IsQuotePost = q
		}
		if rt := tpi["reply_to_author"]; rt != nil {
			p.IsReply = true
			if author, ok := rt.(map[string]any); ok {
				p.ReplyToID = asString(author["pk"])
			}
		}
	}

	if p.Shortcode != "" && p.Username != "" {
		p.Permalink = WebBase + "/@" + p.Username + "/post/" + p.Shortcode
	}
	return p
}

func mergeProfile(p *Profile, u map[string]any) {
	if s := asString(u["pk"]); s != "" {
		p.ID = s
	}
	if s := asString(u["username"]); s != "" {
		p.Username = s
	}
	if s := asString(u["full_name"]); s != "" {
		p.Name = s
	}
	if s := asString(u["biography"]); s != "" {
		p.Biography = s
	}
	if s := asString(u["profile_pic_url"]); s != "" {
		p.ProfilePicURL = s
	}
	if b, ok := u["is_verified"].(bool); ok {
		p.IsVerified = b
		p.VerifiedAvailable = true
	}
	if f, ok := metricInt(u, "follower_count"); ok {
		p.FollowerCount = f
		p.FollowerCountAvailable = true
	}
	if f, ok := metricInt(u, "following_count"); ok {
		p.FollowingCount = f
		p.FollowingCountAvailable = true
	}
}

// ── small value helpers ───────────────────────────────────

func asString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatInt(int64(x), 10)
	}
	return ""
}

func asFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return f
	}
	return 0
}

// metricInt returns a count only when the source map contains a numeric value.
// A present zero is available; a missing or null value is not.
func metricInt(data map[string]any, key string) (int64, bool) {
	v, exists := data[key]
	if !exists || v == nil {
		return 0, false
	}
	return int64(asFloat(v)), true
}

func firstImageURL(data map[string]any) string {
	if iv, ok := data["image_versions2"].(map[string]any); ok {
		if cands, ok := iv["candidates"].([]any); ok && len(cands) > 0 {
			if c, ok := cands[0].(map[string]any); ok {
				return asString(c["url"])
			}
		}
	}
	return ""
}

func firstVideoURL(data map[string]any) string {
	if vv, ok := data["video_versions"].([]any); ok && len(vv) > 0 {
		if v, ok := vv[0].(map[string]any); ok {
			return asString(v["url"])
		}
	}
	return ""
}

func carouselURLs(data map[string]any) []string {
	var out []string
	media, ok := data["carousel_media"].([]any)
	if !ok {
		return nil
	}
	for _, item := range media {
		im, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if u := firstVideoURL(im); u != "" {
			out = append(out, u)
			continue
		}
		if u := firstImageURL(im); u != "" {
			out = append(out, u)
		}
	}
	return out
}

// countFromDescription pulls "<n> followers" style counts (incl. K/M suffixes)
// out of an og:description string.
func countFromDescription(desc, word string) (int64, bool) {
	re := regexp.MustCompile(`([0-9][0-9.,]*)\s*([KkMm]?)\s*` + word)
	m := re.FindStringSubmatch(desc)
	if m == nil {
		return 0, false
	}
	base := strings.NewReplacer(",", "").Replace(m[1])
	f, err := strconv.ParseFloat(base, 64)
	if err != nil {
		return 0, false
	}
	switch strings.ToUpper(m[2]) {
	case "K":
		f *= 1_000
	case "M":
		f *= 1_000_000
	}
	return int64(f), true
}
