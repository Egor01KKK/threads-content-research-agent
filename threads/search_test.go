package threads

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSearchResultPreservesAvailablePostMetrics(t *testing.T) {
	post := Post{
		ID:                   "1",
		Shortcode:            "ABC123",
		Username:             "ada",
		UserID:               "42",
		LikeCount:            10,
		ReplyCount:           2,
		RepostCount:          3,
		QuoteCount:           4,
		ViewCount:            5,
		LikeCountAvailable:   true,
		ReplyCountAvailable:  true,
		RepostCountAvailable: true,
		QuoteCountAvailable:  true,
		ViewCountAvailable:   true,
		FetchedAt:            time.Unix(100, 0).UTC(),
	}
	result := searchResultFromPost(post, "automation", time.Unix(200, 0).UTC())
	if result.Shortcode != post.Shortcode || result.UserID != post.UserID {
		t.Fatalf("identity fields were dropped: %+v", result)
	}
	for name, value := range map[string]*int64{
		"likes":   result.LikeCount,
		"replies": result.ReplyCount,
		"reposts": result.RepostCount,
		"quotes":  result.QuoteCount,
		"views":   result.ViewCount,
	} {
		if value == nil {
			t.Errorf("%s metric is nil", name)
		}
	}
	if *result.LikeCount != 10 || *result.ReplyCount != 2 || *result.RepostCount != 3 || *result.QuoteCount != 4 || *result.ViewCount != 5 {
		t.Errorf("metrics = %+v", result)
	}
}

func TestSearchResultLeavesUnavailableMetricsNil(t *testing.T) {
	result := searchResultFromPost(Post{ID: "1"}, "topic", time.Now())
	if result.LikeCount != nil || result.ReplyCount != nil || result.RepostCount != nil || result.QuoteCount != nil || result.ViewCount != nil {
		t.Fatalf("unavailable metrics must remain nil: %+v", result)
	}
}

const searchFixtureHTML = `<script type="application/json" data-sjs>
{"queryName":"BarcelonaSearchResultsQuery","data":{"searchResults":{"edges":[
  {"node":{"thread":{"thread_items":[{"post":{"pk":"search-1","code":"SEARCH1","caption":{"text":"actual result"},"like_count":8,"user":{"pk":"1","username":"ada"},"text_post_app_info":{"direct_reply_count":2}}}]}}}
]},"recommended":{"thread_items":[{"post":{"pk":"unrelated-1","code":"OTHER1","caption":{"text":"recommended noise"},"like_count":999,"user":{"pk":"2","username":"noise"}}}]}}}
</script>`

func TestParseSearchPostsSSRExcludesUnrelatedThreadItems(t *testing.T) {
	posts, valid := parseSearchPostsSSR(searchFixtureHTML)
	if !valid {
		t.Fatal("search payload was not recognized")
	}
	if len(posts) != 1 || posts[0].ID != "search-1" {
		t.Fatalf("posts = %+v, want only search-1", posts)
	}
}

func TestParseSearchPostsSSRRecognizesEmptyResultPayload(t *testing.T) {
	html := `<script type="application/json" data-sjs>{"queryName":"BarcelonaSearchResultsQuery","data":{"searchResults":{"edges":[]}}}</script>`
	posts, valid := parseSearchPostsSSR(html)
	if !valid || len(posts) != 0 {
		t.Fatalf("empty search payload = posts=%v valid=%v", posts, valid)
	}
}

func TestParseSearchPostsSSRFallsBackForShellWithoutResultsPayload(t *testing.T) {
	html := `<script type="application/json" data-sjs>{"queryName":"BarcelonaSearchResultsQuery","data":{"shell":true}}</script>`
	posts, valid := parseSearchPostsSSR(html)
	if valid || len(posts) != 0 {
		t.Fatalf("shell was mistaken for empty result payload: posts=%v valid=%v", posts, valid)
	}
}

func TestSearchPageInfoSSRReadsSearchCursorOnly(t *testing.T) {
	html := `<script type="application/json" data-sjs>{"queryName":"BarcelonaSearchResultsQuery","data":{"searchResults":{"edges":[],"page_info":{"end_cursor":"SEARCH_CURSOR","has_next_page":true}}}}</script>`
	cursor, more := searchPageInfoSSR(html)
	if cursor != "SEARCH_CURSOR" || !more {
		t.Errorf("search page info = cursor=%q more=%v", cursor, more)
	}
}

func TestSearchShellUsesGraphQLFallback(t *testing.T) {
	var methods []string
	client := &Client{
		cfg: Config{Retries: 1},
		http: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			methods = append(methods, req.Method)
			body := `<script type="application/json" data-sjs>{"queryName":"BarcelonaSearchResultsQuery","data":{"searchResults":{"shell":true}}}</script>`
			if req.Method == http.MethodPost {
				body = `{"data":{"thread_items":[{"post":{"pk":"graphql-1","code":"GQL1","caption":{"text":"fallback result"},"like_count":4,"user":{"pk":"1","username":"ada"}}}]}}`
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
			}, nil
		})},
		cache: NewCache(t.TempDir(), false, time.Hour),
	}
	posts, err := client.searchPosts(context.Background(), "topic")
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 1 || posts[0].ID != "graphql-1" {
		t.Fatalf("fallback posts = %+v", posts)
	}
	if len(methods) != 2 || methods[0] != http.MethodGet || methods[1] != http.MethodPost {
		t.Errorf("request methods = %v, want GET then POST", methods)
	}
}

func TestSearchContinuesSSRWindowThroughGraphQLCursor(t *testing.T) {
	var methods []string
	client := &Client{
		cfg: Config{Retries: 1},
		http: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			methods = append(methods, req.Method)
			body := `<script type="application/json" data-sjs>{"queryName":"BarcelonaSearchResultsQuery","data":{"searchResults":{"edges":[{"node":{"thread":{"thread_items":[{"post":{"pk":"ssr-1","code":"SSR1","caption":{"text":"first result"},"like_count":1,"user":{"pk":"1","username":"ada"}}}]}}}],"page_info":{"end_cursor":"CURSOR","has_next_page":true}}}}</script>`
			if req.Method == http.MethodPost {
				body = `{"data":{"thread_items":[{"post":{"pk":"gql-2","code":"GQL2","caption":{"text":"continued result"},"like_count":2,"user":{"pk":"2","username":"bob"}}}]}}`
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		})},
		cache: NewCache(t.TempDir(), false, time.Hour),
	}
	posts, err := client.searchPosts(context.Background(), "topic")
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 2 || posts[0].ID != "ssr-1" || posts[1].ID != "gql-2" {
		t.Fatalf("continued search posts = %+v", posts)
	}
	if len(methods) != 2 || methods[0] != http.MethodGet || methods[1] != http.MethodPost {
		t.Errorf("request methods = %v, want GET then POST continuation", methods)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
