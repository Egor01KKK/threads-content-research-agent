package threads

import (
	"context"
	"encoding/json"
	"iter"
	"net/url"
	"strings"
	"time"
)

// Search streams keyword search hits from the public server-rendered search
// page. Threads changes the logged-out GraphQL search document frequently, so
// the SSR surface is the primary path and the persisted query remains a
// fallback for older responses or authenticated configurations.
func (c *Client) Search(ctx context.Context, query string, limit int) iter.Seq2[SearchResult, error] {
	return func(yield func(SearchResult, error) bool) {
		posts, err := c.searchPosts(ctx, query)
		if err != nil {
			yield(SearchResult{}, err)
			return
		}
		n := 0
		for _, p := range posts {
			r := searchResultFromPost(p, query, time.Now())
			if !yield(r, nil) {
				return
			}
			n++
			if limit > 0 && n >= limit {
				return
			}
		}
	}
}

func (c *Client) searchPosts(ctx context.Context, query string) ([]Post, error) {
	pageURL := WebBase + "/search?q=" + url.QueryEscape(query)
	html, err := c.getHTML(ctx, pageURL)
	if err == nil {
		if posts, valid := parseSearchPostsSSR(html); valid {
			// The public SSR window can advertise another page. Continue through
			// the same persisted query used by profile pagination, but keep the
			// valid SSR window if the compatibility cursor request is unavailable.
			cursor, hasMore := searchPageInfoSSR(html)
			if hasMore && cursor != "" {
				extra, _ := c.graphqlSearchThreads(ctx, query, cursor)
				// Keep any pages received before a cursor failure. The public SSR
				// window is still valid, and partial continuation is more useful
				// than silently discarding already collected posts.
				posts = appendUniquePosts(posts, extra)
			}
			return posts, nil
		}
	} else if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Keep the existing GraphQL route as a compatibility fallback. It is useful
	// when Threads serves a shell without embedded results and for sessions that
	// still expose the persisted search document.
	posts, gqlErr := c.graphqlSearch(ctx, query)
	if gqlErr != nil {
		if err != nil {
			return nil, err
		}
		return nil, gqlErr
	}
	return posts, nil
}

func searchPageInfoSSR(html string) (cursor string, hasMore bool) {
	for _, raw := range dataSJSBlocks(html) {
		if !strings.Contains(raw, `"searchResults"`) || !strings.Contains(raw, "page_info") {
			continue
		}
		var data any
		if json.Unmarshal([]byte(raw), &data) != nil {
			continue
		}
		for _, results := range findSearchResults(data, 0) {
			pageInfo, ok := results["page_info"].(map[string]any)
			if !ok {
				continue
			}
			cursor, _ = pageInfo["end_cursor"].(string)
			hasMore, _ = pageInfo["has_next_page"].(bool)
			return cursor, hasMore
		}
	}
	return "", false
}

func appendUniquePosts(existing, extra []Post) []Post {
	seen := make(map[string]bool, len(existing)+len(extra))
	for _, post := range existing {
		if post.ID != "" {
			seen[post.ID] = true
		}
	}
	for _, post := range extra {
		if post.ID == "" || seen[post.ID] {
			continue
		}
		seen[post.ID] = true
		existing = append(existing, post)
	}
	return existing
}

func searchResultFromPost(p Post, query string, searchedAt time.Time) SearchResult {
	r := SearchResult{
		ID:          p.ID,
		Query:       query,
		Shortcode:   p.Shortcode,
		Text:        p.Text,
		Username:    p.Username,
		UserID:      p.UserID,
		Permalink:   p.Permalink,
		Timestamp:   p.Timestamp,
		MediaType:   p.MediaType,
		IsReply:     p.IsReply,
		IsQuotePost: p.IsQuotePost,
		FetchedAt:   p.FetchedAt,
		SearchedAt:  searchedAt,
	}
	if p.LikeCountAvailable {
		v := p.LikeCount
		r.LikeCount = &v
	}
	if p.ReplyCountAvailable {
		v := p.ReplyCount
		r.ReplyCount = &v
	}
	if p.RepostCountAvailable {
		v := p.RepostCount
		r.RepostCount = &v
	}
	if p.QuoteCountAvailable {
		v := p.QuoteCount
		r.QuoteCount = &v
	}
	if p.ViewCountAvailable {
		v := p.ViewCount
		r.ViewCount = &v
	}
	return r
}
