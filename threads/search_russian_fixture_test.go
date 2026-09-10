package threads

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseRussianSearchFixturePreservesPostAndMetrics(t *testing.T) {
	path := filepath.Join("testdata", "search_russian_fixture.html")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	posts, valid := parseSearchPostsSSR(string(body))
	if !valid || len(posts) != 1 {
		t.Fatalf("posts=%d valid=%v, want one valid Russian search result", len(posts), valid)
	}
	post := posts[0]
	if post.ID != "401234567890_123456789" || post.Shortcode != "RUSSIAN1" {
		t.Fatalf("identity = %+v", post)
	}
	if post.Text != "Веду заявки вручную и каждый день теряю время. Как это автоматизировать?" {
		t.Fatalf("text = %q", post.Text)
	}
	if post.Username != "smallbiz_ru" || post.UserID != "123456789" {
		t.Fatalf("author = %q/%q", post.Username, post.UserID)
	}
	if post.Permalink != WebBase+"/@smallbiz_ru/post/RUSSIAN1" {
		t.Fatalf("permalink = %q", post.Permalink)
	}
	if !post.LikeCountAvailable || post.LikeCount != 17 || !post.ReplyCountAvailable || post.ReplyCount != 4 {
		t.Fatalf("likes/replies = %d/%d available=%v/%v", post.LikeCount, post.ReplyCount, post.LikeCountAvailable, post.ReplyCountAvailable)
	}
	if !post.RepostCountAvailable || post.RepostCount != 2 || !post.QuoteCountAvailable || post.QuoteCount != 1 {
		t.Fatalf("reposts/quotes = %d/%d available=%v/%v", post.RepostCount, post.QuoteCount, post.RepostCountAvailable, post.QuoteCountAvailable)
	}
}
