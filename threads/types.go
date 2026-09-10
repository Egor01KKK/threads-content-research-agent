package threads

import "time"

// Profile is a Threads user.
type Profile struct {
	ID             string    `json:"id,omitempty"`
	Username       string    `json:"username"`
	Name           string    `json:"name,omitempty"`
	Biography      string    `json:"biography,omitempty"`
	ProfilePicURL  string    `json:"profile_pic_url,omitempty"`
	IsVerified     bool      `json:"is_verified"`
	FollowerCount  int64     `json:"follower_count,omitempty"`
	FollowingCount int64     `json:"following_count,omitempty"`
	URL            string    `json:"url"`
	FetchedAt      time.Time `json:"fetched_at"`

	// Availability flags preserve the difference between an exposed zero and a
	// field that the anonymous surface did not expose. They are omitted from
	// normal JSON output when false so existing command output stays compatible.
	FollowerCountAvailable  bool `json:"follower_count_available,omitempty"`
	FollowingCountAvailable bool `json:"following_count_available,omitempty"`
	VerifiedAvailable       bool `json:"verified_available,omitempty"`
}

// Post is a single Threads post.
type Post struct {
	ID           string    `json:"id"`
	Shortcode    string    `json:"shortcode,omitempty"`
	Text         string    `json:"text,omitempty"`
	MediaType    string    `json:"media_type,omitempty"` // TEXT_POST, IMAGE, VIDEO, CAROUSEL_ALBUM
	MediaURLs    []string  `json:"media_urls,omitempty"`
	Permalink    string    `json:"permalink,omitempty"`
	Username     string    `json:"username,omitempty"`
	UserID       string    `json:"user_id,omitempty"`
	Timestamp    time.Time `json:"timestamp,omitempty"`
	LikeCount    int64     `json:"like_count"`
	ReplyCount   int64     `json:"reply_count"`
	RepostCount  int64     `json:"repost_count"`
	QuoteCount   int64     `json:"quote_count"`
	ViewCount    int64     `json:"view_count,omitempty"`
	IsQuotePost  bool      `json:"is_quote_post,omitempty"`
	IsReply      bool      `json:"is_reply,omitempty"`
	QuotedPostID string    `json:"quoted_post_id,omitempty"`
	ReplyToID    string    `json:"reply_to_id,omitempty"`
	HasMedia     bool      `json:"has_media,omitempty"`
	FetchedAt    time.Time `json:"fetched_at"`

	// Count availability is intentionally additive. Existing callers can keep
	// using the int64 fields, while research storage can write SQL NULL when a
	// metric was absent rather than mistaking it for zero.
	LikeCountAvailable   bool `json:"like_count_available,omitempty"`
	ReplyCountAvailable  bool `json:"reply_count_available,omitempty"`
	RepostCountAvailable bool `json:"repost_count_available,omitempty"`
	QuoteCountAvailable  bool `json:"quote_count_available,omitempty"`
	ViewCountAvailable   bool `json:"view_count_available,omitempty"`
}

// Reply is a reply to a post, carrying its thread linkage.
type Reply struct {
	ID         string    `json:"id"`
	ParentID   string    `json:"parent_id,omitempty"`
	RootID     string    `json:"root_id,omitempty"`
	Shortcode  string    `json:"shortcode,omitempty"`
	Text       string    `json:"text,omitempty"`
	Username   string    `json:"username,omitempty"`
	UserID     string    `json:"user_id,omitempty"`
	Permalink  string    `json:"permalink,omitempty"`
	Timestamp  time.Time `json:"timestamp,omitempty"`
	LikeCount  int64     `json:"like_count"`
	ReplyCount int64     `json:"reply_count"`
	MediaType  string    `json:"media_type,omitempty"`
	MediaURLs  []string  `json:"media_urls,omitempty"`
	FetchedAt  time.Time `json:"fetched_at"`
}

// SearchResult is one hit from a keyword search.
type SearchResult struct {
	ID          string    `json:"id"`
	Query       string    `json:"query"`
	Shortcode   string    `json:"shortcode,omitempty"`
	Text        string    `json:"text,omitempty"`
	Username    string    `json:"username,omitempty"`
	UserID      string    `json:"user_id,omitempty"`
	Permalink   string    `json:"permalink,omitempty"`
	Timestamp   time.Time `json:"timestamp,omitempty"`
	MediaType   string    `json:"media_type,omitempty"`
	LikeCount   *int64    `json:"like_count,omitempty"`
	ReplyCount  *int64    `json:"reply_count,omitempty"`
	RepostCount *int64    `json:"repost_count,omitempty"`
	QuoteCount  *int64    `json:"quote_count,omitempty"`
	ViewCount   *int64    `json:"view_count,omitempty"`
	IsReply     bool      `json:"is_reply,omitempty"`
	IsQuotePost bool      `json:"is_quote_post,omitempty"`
	FetchedAt   time.Time `json:"fetched_at,omitempty"`
	SearchedAt  time.Time `json:"searched_at"`
}

// asReply converts a parsed post (a reply lives in the same thread_items shape
// as a post) into a Reply under the given root/parent.
func (p Post) asReply(parentID, rootID string) Reply {
	return Reply{
		ID:         p.ID,
		ParentID:   parentID,
		RootID:     rootID,
		Shortcode:  p.Shortcode,
		Text:       p.Text,
		Username:   p.Username,
		UserID:     p.UserID,
		Permalink:  p.Permalink,
		Timestamp:  p.Timestamp,
		LikeCount:  p.LikeCount,
		ReplyCount: p.ReplyCount,
		MediaType:  p.MediaType,
		MediaURLs:  p.MediaURLs,
		FetchedAt:  p.FetchedAt,
	}
}
