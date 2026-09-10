// Package threads is the library behind the th command line: the crawler HTTP
// client, the server-rendered-page parsers, the logged-out GraphQL queries, and
// the typed data models for Threads (threads.com).
//
// The default transport presents a crawler user agent, which is what makes
// Threads answer with server-rendered HTML that carries the post text, the
// engagement counts, the media, and a window of replies. No login, no cookie,
// and no browser are involved. The optional session mode can add depth on top
// of that anonymous floor when the caller supplies their own credentials.
package threads

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// Client speaks the Threads web surface as a crawler.
type Client struct {
	cfg     Config
	http    *http.Client
	cache   *Cache
	mu      sync.Mutex
	lastReq time.Time
}

// NewClient builds a Client from cfg, resolving the proxy.
func NewClient(cfg Config) (*Client, error) {
	transport := &http.Transport{
		MaxIdleConns:        16,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	if cfg.Proxy != "" {
		pu, perr := url.Parse(cfg.Proxy)
		if perr != nil {
			return nil, codeErr(ExitUsage, "bad proxy URL: %v", perr)
		}
		transport.Proxy = http.ProxyURL(pu)
	}
	return &Client{
		cfg:   cfg,
		http:  &http.Client{Timeout: cfg.Timeout, Transport: transport},
		cache: NewCache(cfg.CacheDir, !cfg.NoCache, cfg.CacheTTL),
	}, nil
}

// UserAgent reports the crawler identity requests are sent with.
func (c *Client) UserAgent() string { return c.cfg.UserAgent }

// Cache exposes the client's blob cache (for the cache command).
func (c *Client) Cache() *Cache { return c.cache }

// Mode reports the access mode: "anonymous" or "session".
func (c *Client) Mode() string {
	switch {
	case c.cfg.Session != "" && c.cfg.CSRF != "":
		return "session"
	default:
		return "anonymous"
	}
}

// getHTML fetches a page as the crawler and returns the body as a string, with
// login walls and error shells mapped to typed CodeErrors.
func (c *Client) getHTML(ctx context.Context, rawURL string) (string, error) {
	b, err := c.getBytes(ctx, rawURL)
	return string(b), err
}

// GetRaw returns the raw crawler response body for --raw.
func (c *Client) GetRaw(ctx context.Context, rawURL string) ([]byte, error) {
	return c.getBytes(ctx, rawURL)
}

func (c *Client) getBytes(ctx context.Context, target string) ([]byte, error) {
	if b, ok := c.cache.Get(target); ok {
		c.logf(2, "cache hit %s", target)
		if w := wallOrShell(b); w != nil {
			return b, w
		}
		return b, nil
	}
	body, err := c.fetch(ctx, target)
	if err != nil {
		return body, err
	}
	if w := wallOrShell(body); w != nil {
		return body, w
	}
	c.cache.Put(target, body)
	return body, nil
}

func wallOrShell(body []byte) *CodeError {
	if isLoginWall(body) {
		return errLoginWall()
	}
	if isErrorShell(body) {
		return codeErr(ExitNotFound, "Threads returned an error page; the content may be unavailable or private")
	}
	return nil
}

func (c *Client) fetch(ctx context.Context, target string) ([]byte, error) {
	attempts := c.cfg.Retries
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		c.rateLimit()
		body, code, err := c.doGet(ctx, target)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = err
			if attempt == attempts {
				return nil, codeErr(ExitNetwork, "request failed after %d attempts: %v", attempts, err)
			}
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}
		switch {
		case code == 404:
			return body, errNotFound(target)
		case code == 429 || code == 503:
			if attempt == attempts {
				return body, codeErr(ExitRateLimit, "rate limited (HTTP %d) after %d attempts", code, attempts)
			}
			time.Sleep(time.Duration(attempt*attempt) * 3 * time.Second)
			continue
		case code >= 500:
			if attempt == attempts {
				return body, codeErr(ExitNetwork, "server error HTTP %d", code)
			}
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
			continue
		}
		return body, nil
	}
	return nil, codeErr(ExitNetwork, "all attempts failed: %v", lastErr)
}

func (c *Client) doGet(ctx context.Context, target string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	lang := c.cfg.Lang
	if lang == "" {
		lang = "en-US"
	}
	req.Header.Set("Accept-Language", lang+","+strings.SplitN(lang, "-", 2)[0]+";q=0.9")
	c.logf(2, "GET %s", target)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

func (c *Client) rateLimit() {
	if c.cfg.Delay <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if since := time.Since(c.lastReq); since < c.cfg.Delay {
		time.Sleep(c.cfg.Delay - since)
	}
	c.lastReq = time.Now()
}

func (c *Client) logf(level int, format string, args ...any) {
	if c.cfg.Verbose >= level {
		fmt.Fprintf(os.Stderr, "[th] "+format+"\n", args...)
	}
}

func isLoginWall(body []byte) bool {
	s := strings.ToLower(string(body))
	return strings.Contains(s, "log in to threads") ||
		strings.Contains(s, "log in to see") ||
		strings.Contains(s, "you must log in to continue") ||
		strings.Contains(s, "this account is private")
}

func isErrorShell(body []byte) bool {
	s := strings.ToLower(string(body))
	return strings.Contains(s, "sorry, this page isn't available") ||
		strings.Contains(s, "the link you followed may be broken") ||
		strings.Contains(s, "page not found")
}
