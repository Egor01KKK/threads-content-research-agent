package threads

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strings"
)

// The logged-out GraphQL path. Threads marks a caller as a crawler through a set
// of relay provider flags; with those set, the persisted profile-threads, post,
// and search queries return data without a session. doc_id values rotate (see
// config.go), so a stale id degrades to "no extra data" rather than an error.

func relayProviderVars() map[string]any {
	return map[string]any{
		"__relay_internal__pv__BarcelonaIsLoggedInrelayprovider":             false,
		"__relay_internal__pv__BarcelonaIsInternalUserrelayprovider":         false,
		"__relay_internal__pv__BarcelonaIsCrawlerrelayprovider":              true,
		"__relay_internal__pv__BarcelonaOptionalCookiesEnabledrelayprovider": true,
		"__relay_internal__pv__BarcelonaIsLoggedOutrelayprovider":            true,
	}
}

// maxGraphQLPages caps how far the logged-out pagination walks, so an unbounded
// crawl cannot loop forever on a profile with a very long history.
const maxGraphQLPages = 20

// graphqlProfileThreads walks a user's posts via the logged-out persisted query,
// following the page_info cursor from startCursor until it runs out or the page
// cap is hit. startCursor is the end_cursor from the server-rendered window, so
// pagination resumes where the SSR page left off.
func (c *Client) graphqlProfileThreads(ctx context.Context, userID, startCursor string) ([]Post, error) {
	var out []Post
	cursor := startCursor
	for page := 0; page < maxGraphQLPages; page++ {
		vars := map[string]any{"userID": userID}
		if cursor != "" {
			vars["after"] = cursor
		}
		raw, err := c.graphqlPost(ctx, DocIDProfileThreads, vars)
		if err != nil {
			return out, err
		}
		out = append(out, postsFromGraphQL(raw)...)
		next, more, ok := findPageInfo(raw, 0)
		if !ok || !more || next == "" || next == cursor {
			break
		}
		cursor = next
	}
	return out, nil
}

// graphqlPostReplies fetches a window of a post's replies via the logged-out
// persisted query.
func (c *Client) graphqlPostReplies(ctx context.Context, postID string) ([]Post, error) {
	vars := map[string]any{"postID": postID}
	raw, err := c.graphqlPost(ctx, DocIDPostPage, vars)
	if err != nil {
		return nil, err
	}
	return postsFromGraphQL(raw), nil
}

// graphqlSearch runs the logged-out keyword search query.
func (c *Client) graphqlSearch(ctx context.Context, query string) ([]Post, error) {
	return c.graphqlSearchThreads(ctx, query, "")
}

// graphqlSearchThreads continues a search result window through its cursor.
// The same hard page cap as profile pagination keeps a malformed or stale
// cursor from turning one bounded search into an unbounded crawl.
func (c *Client) graphqlSearchThreads(ctx context.Context, query, startCursor string) ([]Post, error) {
	var out []Post
	cursor := startCursor
	for page := 0; page < maxGraphQLPages; page++ {
		vars := map[string]any{"query": query}
		if cursor != "" {
			vars["after"] = cursor
		}
		raw, err := c.graphqlPost(ctx, DocIDSearch, vars)
		if err != nil {
			return out, err
		}
		out = appendUniquePosts(out, postsFromGraphQL(raw))
		next, more, ok := findPageInfo(raw, 0)
		if !ok || !more || next == "" || next == cursor {
			break
		}
		cursor = next
	}
	return out, nil
}

// graphqlPost POSTs a persisted query and returns the decoded data tree.
func (c *Client) graphqlPost(ctx context.Context, docID string, vars map[string]any) (any, error) {
	maps.Copy(vars, relayProviderVars())
	varsJSON, _ := json.Marshal(vars)
	form := url.Values{}
	form.Set("lsd", "t")
	form.Set("doc_id", docID)
	form.Set("variables", string(varsJSON))

	c.rateLimit()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, GraphQLURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-FB-LSD", "t")
	req.Header.Set("X-IG-App-ID", "238260118697367")
	if c.cfg.Session != "" {
		req.Header.Set("Cookie", "sessionid="+c.cfg.Session)
	}
	if c.cfg.CSRF != "" {
		req.Header.Set("X-CSRFToken", c.cfg.CSRF)
	}
	c.logf(2, "POST graphql doc_id=%s", docID)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, codeErr(ExitNetwork, "graphql request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, codeErr(ExitNotFound, "graphql returned an unexpected shape (doc_id may be stale)")
	}
	var data any
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return nil, codeErr(ExitNotFound, "graphql returned an unexpected shape (doc_id may be stale)")
	}
	return data, nil
}

// postsFromGraphQL reuses the SSR thread_items walker over the GraphQL data
// tree: both surfaces nest the same post objects.
func postsFromGraphQL(data any) []Post {
	posts := walkThreadItems(data, 0)
	out := posts[:0]
	seen := map[string]bool{}
	for _, p := range posts {
		if p.ID == "" || seen[p.ID] {
			continue
		}
		seen[p.ID] = true
		out = append(out, p)
	}
	return out
}
