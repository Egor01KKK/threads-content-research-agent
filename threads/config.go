package threads

import (
	"os"
	"path/filepath"
	"time"
)

// Default request parameters.
const (
	DefaultDelay   = 1 * time.Second
	DefaultRetries = 4
	DefaultTimeout = 30 * time.Second
)

// CrawlerUA is the user agent that makes Threads serve server-rendered HTML.
// Presenting as a crawler is what unlocks anonymous, no-browser access.
const CrawlerUA = "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"

// Web and API hosts. The crawler surface lives on threads.com; the official
// Graph API on graph.threads.net.
const (
	WebBase    = "https://www.threads.com"
	GraphQLURL = "https://www.threads.com/api/graphql"
	APIBase    = "https://graph.threads.net/v1.0"
)

// doc_id values for the logged-out persisted queries. Threads rotates these
// every two to four weeks; when a query starts returning an unexpected shape,
// refresh these from a logged-out page load and the anonymous pagination path
// recovers. The SSR path does not depend on them.
const (
	DocIDProfileThreads = "33773912952222602" // a profile's threads tab
	DocIDPostPage       = "7448594591874178"  // a single post page and its replies
	DocIDSearch         = "24871030029227550" // keyword/user search
)

// Config is the resolved runtime configuration for a Client.
type Config struct {
	Delay     time.Duration
	Retries   int
	Timeout   time.Duration
	UserAgent string
	Proxy     string
	Lang      string
	CacheDir  string
	NoCache   bool
	CacheTTL  time.Duration
	DataDir   string
	Verbose   int

	// Optional session depth, off by default.
	Session string // logged-in session id cookie
	CSRF    string // session CSRF token
}

// DefaultConfig returns the built-in defaults with XDG paths filled in and the
// optional credentials read from the environment.
func DefaultConfig() Config {
	return Config{
		Delay:     DefaultDelay,
		Retries:   DefaultRetries,
		Timeout:   DefaultTimeout,
		UserAgent: CrawlerUA,
		Lang:      "en-US",
		CacheDir:  filepath.Join(cacheHome(), "th"),
		CacheTTL:  time.Hour,
		DataDir:   filepath.Join(dataHome(), "th"),
		Session:   os.Getenv("THREADS_SESSION"),
		CSRF:      os.Getenv("THREADS_CSRF"),
	}
}

func cacheHome() string {
	if d := os.Getenv("XDG_CACHE_HOME"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache")
}

func dataHome() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share")
}
