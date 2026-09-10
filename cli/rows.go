package cli

import (
	"strconv"
	"strings"

	"github.com/Egor01KKK/threads-content-research-agent/pkg/thid"
	"github.com/Egor01KKK/threads-content-research-agent/threads"
)

func i64(v int64) string { return strconv.FormatInt(v, 10) }

func profileRow(p *threads.Profile) Row {
	return Row{
		Cols:  []string{"id", "username", "name", "followers", "following", "verified", "bio", "url"},
		Vals:  []string{p.ID, p.Username, p.Name, i64(p.FollowerCount), i64(p.FollowingCount), strconv.FormatBool(p.IsVerified), truncate(p.Biography, 80), p.URL},
		Value: p,
	}
}

func postRow(p *threads.Post) Row {
	return Row{
		Cols:  []string{"id", "username", "timestamp", "likes", "replies", "reposts", "media", "text", "permalink"},
		Vals:  []string{p.ID, p.Username, tsText(p), i64(p.LikeCount), i64(p.ReplyCount), i64(p.RepostCount), p.MediaType, truncate(p.Text, 80), p.Permalink},
		Value: p,
	}
}

func replyRow(r *threads.Reply) Row {
	return Row{
		Cols:  []string{"id", "username", "likes", "replies", "text", "parent_id", "root_id", "permalink"},
		Vals:  []string{r.ID, r.Username, i64(r.LikeCount), i64(r.ReplyCount), truncate(r.Text, 80), r.ParentID, r.RootID, r.Permalink},
		Value: r,
	}
}

func searchRow(r *threads.SearchResult) Row {
	return Row{
		Cols:  []string{"id", "username", "text", "media", "permalink"},
		Vals:  []string{r.ID, r.Username, truncate(r.Text, 80), r.MediaType, r.Permalink},
		Value: r,
	}
}

func identityRow(id thid.Identity) Row {
	return Row{
		Cols:  []string{"kind", "handle", "shortcode", "pk", "url"},
		Vals:  []string{string(id.Kind), id.Handle, id.Shortcode, id.PK, id.URL},
		Value: id,
	}
}

func tsText(p *threads.Post) string {
	if p.Timestamp.IsZero() {
		return ""
	}
	return p.Timestamp.UTC().Format("2006-01-02 15:04")
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
