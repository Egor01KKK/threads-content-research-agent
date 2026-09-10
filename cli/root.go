// Package cli builds the th command tree on top of the threads library.
package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Egor01KKK/threads-content-research-agent/threads"
	"github.com/spf13/cobra"
)

// Build metadata, set via -ldflags at release time.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// App carries the resolved configuration and shared client for a command run.
type App struct {
	Cfg    threads.Config
	Client *threads.Client
	Out    *Output
	Limit  int
	g      *globalFlags
}

// globalFlags holds the persistent flag values before they fold into Cfg.
type globalFlags struct {
	output    string
	fields    string
	noHeader  bool
	template  string
	limit     int
	rate      time.Duration
	retries   int
	timeout   time.Duration
	noCache   bool
	cacheTTL  time.Duration
	lang      string
	quiet     bool
	verbose   int
	proxy     string
	userAgent string
	session   string
	csrf      string
}

// Root builds the root command and its whole subtree.
func Root() *cobra.Command {
	g := &globalFlags{}
	app := &App{g: g}

	root := &cobra.Command{
		Use:   "th",
		Short: "A delightful command line for Threads",
		Long: `th turns threads.com into a fast, scriptable command line.

Resolve a profile to a rich record; stream its recent posts and replies; pull a
single post's thread; search; and build datasets, all from one binary. Reads are
anonymous: th crawls the same server-rendered pages Threads serves to search
engines, so there is no login and no browser.

Quick start:
  th profile zuck                    a profile's full record
  th profile zuck --posts -n 20      its twenty most recent posts
  th post <url> --replies            a post and its reply thread
  th id <anything>                   classify any Threads handle, id, or URL`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Name() == "semantic-audit" {
				app.Limit = g.limit
				app.Out = newOutput(g)
				return nil
			}
			return app.init(g)
		},
	}
	// Map a bad flag to the usage exit code, the same as a bad argument count.
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return threads.Usagef("%s", err)
	})

	pf := root.PersistentFlags()
	pf.StringVarP(&g.output, "output", "o", "auto", "table|json|jsonl|csv|tsv|yaml|url|raw")
	pf.StringVar(&g.fields, "fields", "", "comma-separated columns to keep/order")
	pf.BoolVar(&g.noHeader, "no-header", false, "omit the header row (table/csv/tsv)")
	pf.StringVar(&g.template, "template", "", "Go text/template applied per record")
	pf.IntVarP(&g.limit, "limit", "n", 0, "max records emitted (0 = unlimited)")
	pf.DurationVar(&g.rate, "delay", threads.DefaultDelay, "min delay between requests")
	pf.IntVar(&g.retries, "retries", threads.DefaultRetries, "retry attempts on 429/5xx")
	pf.DurationVar(&g.timeout, "timeout", threads.DefaultTimeout, "per-request timeout")
	pf.BoolVar(&g.noCache, "no-cache", false, "bypass the on-disk cache")
	pf.DurationVar(&g.cacheTTL, "cache-ttl", time.Hour, "cache freshness window")
	pf.StringVar(&g.lang, "lang", "en-US", "Accept-Language / locale")
	pf.BoolVarP(&g.quiet, "quiet", "q", false, "suppress progress on stderr")
	pf.CountVarP(&g.verbose, "verbose", "v", "increase verbosity (repeatable)")
	pf.StringVar(&g.proxy, "proxy", "", "HTTP/SOCKS proxy URL")
	pf.StringVar(&g.userAgent, "user-agent", "", "override the default crawler UA")
	pf.StringVar(&g.session, "session", "", "logged-in session id (or THREADS_SESSION)")
	pf.StringVar(&g.csrf, "csrf", "", "session CSRF token (or THREADS_CSRF)")

	root.AddCommand(
		newProfileCmd(app),
		newPostCmd(app),
		newRepliesCmd(app),
		newFeedCmd(app),
		newSearchCmd(app),
		newResearchCmd(app),
		newSemanticAuditCmd(),
		newIDCmd(app),
		newDBCmd(app),
		newWhoamiCmd(app),
		newConfigCmd(app),
		newCacheCmd(app),
		newVersionCmd(),
	)
	return root
}

func (a *App) init(g *globalFlags) error {
	cfg := threads.DefaultConfig()
	cfg.Delay = g.rate
	cfg.Retries = g.retries
	cfg.Timeout = g.timeout
	cfg.NoCache = g.noCache
	cfg.CacheTTL = g.cacheTTL
	cfg.Lang = g.lang
	cfg.Proxy = g.proxy
	cfg.Verbose = g.verbose
	if g.userAgent != "" {
		cfg.UserAgent = g.userAgent
	}
	if g.session != "" {
		cfg.Session = g.session
	}
	if g.csrf != "" {
		cfg.CSRF = g.csrf
	}

	client, err := threads.NewClient(cfg)
	if err != nil {
		return err
	}
	a.Cfg = cfg
	a.Client = client
	a.Limit = g.limit
	a.Out = newOutput(g)
	return nil
}

// progress writes a status line to stderr unless --quiet is set.
func (a *App) progress(format string, args ...any) {
	if a.g != nil && a.g.quiet {
		return
	}
	_, _ = fmt.Fprintf(os.Stderr, "[th] "+format+"\n", args...)
}

// exactArgs is cobra.ExactArgs with the usage exit code, so a wrong argument
// count exits 2 like every other command-line misuse.
func exactArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != n {
			return threads.Usagef("%q needs exactly %d argument(s), got %d", cmd.CommandPath(), n, len(args))
		}
		return nil
	}
}

// minArgs is cobra.MinimumNArgs with the usage exit code.
func minArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < n {
			return threads.Usagef("%q needs at least %d argument(s), got %d", cmd.CommandPath(), n, len(args))
		}
		return nil
	}
}

// readArgsOrStdin returns args, or lines from stdin when the single arg is "-".
func readArgsOrStdin(args []string) []string {
	if len(args) == 1 && args[0] == "-" {
		var out []string
		sc := bufio.NewScanner(os.Stdin)
		sc.Buffer(make([]byte, 1024*1024), 1024*1024)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line != "" {
				out = append(out, line)
			}
		}
		return out
	}
	return args
}
