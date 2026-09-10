package threads

import (
	"context"
	"iter"

	"github.com/Egor01KKK/threads-content-research-agent/pkg/thid"
)

// postURL resolves any post input to its crawler permalink. A bare shortcode
// without a handle cannot be turned into a permalink, so the caller must pass a
// full URL or a handle/shortcode pair.
func postURL(input string) (string, *CodeError) {
	id := thid.Classify(input)
	if id.Kind != thid.KindPost {
		return "", codeErr(ExitUsage, "not a post: %q", input)
	}
	if id.URL != "" {
		return id.URL, nil
	}
	return "", codeErr(ExitUsage, "need a full post URL (handle and shortcode) for %q", input)
}

// Post fetches a single post in full from its crawler-rendered page.
func (c *Client) Post(ctx context.Context, input string) (*Post, error) {
	target, cerr := postURL(input)
	if cerr != nil {
		return nil, cerr
	}
	html, err := c.getHTML(ctx, target)
	if err != nil {
		return nil, err
	}
	posts := parsePostsSSR(html)
	if len(posts) == 0 {
		return nil, errNotFound("post " + input)
	}
	main := posts[0]
	return &main, nil
}

// PostReplies streams the replies to a post. The crawler page carries the post
// followed by a window of its replies; the logged-out GraphQL query extends that
// window where the doc_id is current.
func (c *Client) PostReplies(ctx context.Context, input string, limit int) iter.Seq2[Reply, error] {
	return func(yield func(Reply, error) bool) {
		target, cerr := postURL(input)
		if cerr != nil {
			yield(Reply{}, cerr)
			return
		}
		html, err := c.getHTML(ctx, target)
		if err != nil {
			yield(Reply{}, err)
			return
		}
		posts := parsePostsSSR(html)
		if len(posts) == 0 {
			yield(Reply{}, errNotFound("post "+input))
			return
		}
		root := posts[0]
		seen := map[string]bool{root.ID: true}
		n := 0
		emit := func(p Post) bool {
			if p.ID == "" || seen[p.ID] {
				return true
			}
			seen[p.ID] = true
			if !yield(p.asReply(root.ID, root.ID), nil) {
				return false
			}
			n++
			return limit <= 0 || n < limit
		}
		for _, p := range posts[1:] {
			if !emit(p) {
				return
			}
		}
		if limit > 0 && n >= limit {
			return
		}
		// Extend via GraphQL.
		more, err := c.graphqlPostReplies(ctx, root.ID)
		if err != nil {
			c.logf(1, "graphql replies: %v", err)
			return
		}
		for _, p := range more {
			if !emit(p) {
				return
			}
		}
	}
}
