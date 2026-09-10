package threads

import (
	"context"
	"iter"

	"github.com/Egor01KKK/threads-content-research-agent/pkg/thid"
)

// Profile fetches a user's profile from the crawler-rendered page.
func (c *Client) Profile(ctx context.Context, handleOrURL string) (*Profile, error) {
	id := thid.Classify(handleOrURL)
	handle := id.Handle
	if handle == "" {
		return nil, codeErr(ExitUsage, "could not read a handle from %q", handleOrURL)
	}
	html, err := c.getHTML(ctx, WebBase+"/@"+handle)
	if err != nil {
		return nil, err
	}
	p := parseProfileSSR(html, handle)
	if p == nil {
		return nil, errNotFound("profile @" + handle)
	}
	return p, nil
}

// ProfilePosts streams a profile's recent posts. It yields the SSR window first,
// then continues through the logged-out GraphQL query while the cursor advances
// and limit (0 = unbounded) is unmet.
func (c *Client) ProfilePosts(ctx context.Context, handleOrURL string, limit int) iter.Seq2[Post, error] {
	return func(yield func(Post, error) bool) {
		id := thid.Classify(handleOrURL)
		handle := id.Handle
		if handle == "" {
			yield(Post{}, codeErr(ExitUsage, "could not read a handle from %q", handleOrURL))
			return
		}
		html, err := c.getHTML(ctx, WebBase+"/@"+handle)
		if err != nil {
			yield(Post{}, err)
			return
		}

		seen := map[string]bool{}
		n := 0
		emit := func(p Post) bool {
			if p.ID == "" || seen[p.ID] {
				return true
			}
			seen[p.ID] = true
			n++
			if !yield(p, nil) {
				return false
			}
			return limit <= 0 || n < limit
		}

		for _, p := range parsePostsSSR(html) {
			if p.IsReply {
				continue
			}
			if !emit(p) {
				return
			}
		}
		if limit > 0 && n >= limit {
			return
		}

		// Extend past the SSR window via the logged-out GraphQL query, keyed by
		// the profile's numeric id.
		prof := parseProfileSSR(html, handle)
		if prof == nil || prof.ID == "" {
			return
		}
		cursor, _ := pageInfoSSR(html)
		posts, err := c.graphqlProfileThreads(ctx, prof.ID, cursor)
		if err != nil {
			// Non-fatal: the SSR window already yielded what it had.
			c.logf(1, "graphql pagination: %v", err)
			return
		}
		for _, p := range posts {
			if p.IsReply {
				continue
			}
			if !emit(p) {
				return
			}
		}
	}
}

// ProfileReplies streams the posts on a profile that are replies.
func (c *Client) ProfileReplies(ctx context.Context, handleOrURL string, limit int) iter.Seq2[Post, error] {
	return func(yield func(Post, error) bool) {
		id := thid.Classify(handleOrURL)
		handle := id.Handle
		if handle == "" {
			yield(Post{}, codeErr(ExitUsage, "could not read a handle from %q", handleOrURL))
			return
		}
		html, err := c.getHTML(ctx, WebBase+"/@"+handle+"/replies")
		if err != nil {
			yield(Post{}, err)
			return
		}
		n := 0
		for _, p := range parsePostsSSR(html) {
			if !p.IsReply {
				continue
			}
			if !yield(p, nil) {
				return
			}
			n++
			if limit > 0 && n >= limit {
				return
			}
		}
	}
}
