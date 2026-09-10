package threads

import (
	"strings"
	"testing"
)

// A trimmed page that mirrors the real shape: og meta tags plus one data-sjs
// block carrying a user object and a thread_items list with one top-level post
// and one reply. The reply has a reply_to_author object; the top-level post
// carries reply_to_author as JSON null, which must NOT be read as a reply.
const fixtureHTML = `<html><head>
<meta property="og:title" content="Ada Lovelace (@ada) on Threads">
<meta property="og:description" content="1,234 Followers. Math and machines.">
<meta property="og:image" content="https://example.com/ada.jpg">
</head><body>
<script type="application/json" data-sjs>
{"require":[["X","next",[],[{"__bbox":{"result":{"data":{"user":{
  "pk":"42","username":"ada","full_name":"Ada Lovelace","biography":"Math and machines.",
  "is_verified":true,"follower_count":1234,"following_count":7},
  "thread_items":[
    {"post":{"pk":"100","code":"ABC","caption":{"text":"first post"},"taken_at":1700000000,
      "like_count":10,"media_type":19,"user":{"pk":"42","username":"ada"},
      "text_post_app_info":{"direct_reply_count":2,"repost_count":1,"quote_count":0,"reply_to_author":null}}},
    {"post":{"pk":"101","code":"DEF","caption":{"text":"a reply"},"taken_at":1700000100,
      "like_count":3,"media_type":19,"user":{"pk":"99","username":"babbage"},
      "text_post_app_info":{"direct_reply_count":0,"reply_to_author":{"pk":"42","username":"ada"}}}}
  ],
  "page_info":{"end_cursor":"CURSOR1","has_next_page":true}}}}}]]]}
</script>
</body></html>`

func TestParseProfileSSR(t *testing.T) {
	p := parseProfileSSR(fixtureHTML, "ada")
	if p == nil {
		t.Fatal("nil profile")
	}
	if p.ID != "42" || p.Username != "ada" || p.Name != "Ada Lovelace" {
		t.Errorf("identity: id=%q user=%q name=%q", p.ID, p.Username, p.Name)
	}
	if !p.IsVerified {
		t.Error("expected verified")
	}
	if p.FollowerCount != 1234 || p.FollowingCount != 7 {
		t.Errorf("counts: followers=%d following=%d", p.FollowerCount, p.FollowingCount)
	}
}

// The logged-out wall Threads serves for a handle with no public profile: og
// tags with the site's generic description, and no embedded user object. It must
// parse to no profile, not to a record carrying the boilerplate as a bio.
const wallHTML = `<html><head>
<meta property="og:title" content="Threads">
<meta property="og:description" content="Join Threads to share ideas, ask questions, post random thoughts, find your people and more.">
</head><body></body></html>`

func TestParseProfileSSRWallIsNotFound(t *testing.T) {
	if p := parseProfileSSR(wallHTML, "nobody_zzqq99"); p != nil {
		t.Errorf("walled page must parse to nil, got %+v", p)
	}
}

func TestParsePostsSSRReplyDetection(t *testing.T) {
	posts := parsePostsSSR(fixtureHTML)
	if len(posts) != 2 {
		t.Fatalf("want 2 posts, got %d", len(posts))
	}
	byID := map[string]Post{}
	for _, p := range posts {
		byID[p.ID] = p
	}
	top, ok := byID["100"]
	if !ok {
		t.Fatal("missing top-level post 100")
	}
	if top.IsReply {
		t.Error("post 100 has reply_to_author=null and must not be a reply")
	}
	if top.Text != "first post" || top.LikeCount != 10 || top.ReplyCount != 2 {
		t.Errorf("post 100 fields: %+v", top)
	}
	if top.Permalink != WebBase+"/@ada/post/ABC" {
		t.Errorf("post 100 permalink: %q", top.Permalink)
	}
	reply, ok := byID["101"]
	if !ok {
		t.Fatal("missing reply 101")
	}
	if !reply.IsReply {
		t.Error("post 101 has a reply_to_author object and must be a reply")
	}
	if reply.ReplyToID != "42" {
		t.Errorf("reply 101 reply_to_id: %q", reply.ReplyToID)
	}
}

func TestPageInfoSSR(t *testing.T) {
	cursor, more := pageInfoSSR(fixtureHTML)
	if cursor != "CURSOR1" || !more {
		t.Errorf("page info: cursor=%q more=%v", cursor, more)
	}
}

func TestMetricPresencePreservesExposedZero(t *testing.T) {
	html := strings.ReplaceAll(fixtureHTML, `"following_count":7`, `"following_count":0`)
	html = strings.ReplaceAll(html, `"like_count":10`, `"like_count":0,"view_count":0`)

	profile := parseProfileSSR(html, "ada")
	if profile == nil {
		t.Fatal("nil profile")
	}
	if profile.FollowingCount != 0 || !profile.FollowingCountAvailable {
		t.Errorf("following count presence: value=%d available=%v", profile.FollowingCount, profile.FollowingCountAvailable)
	}
	posts := parsePostsSSR(html)
	if len(posts) == 0 {
		t.Fatal("no posts")
	}
	post := posts[0]
	if post.LikeCount != 0 || !post.LikeCountAvailable {
		t.Errorf("like count presence: value=%d available=%v", post.LikeCount, post.LikeCountAvailable)
	}
	if post.ViewCount != 0 || !post.ViewCountAvailable {
		t.Errorf("view count presence: value=%d available=%v", post.ViewCount, post.ViewCountAvailable)
	}
}
