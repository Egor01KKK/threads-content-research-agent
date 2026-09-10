package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newWhoamiCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Report how th is accessing Threads",
		RunE: func(cmd *cobra.Command, _ []string) error {
			defer func() { _ = a.Out.Flush() }()
			mode := a.Client.Mode()
			ua := a.Client.UserAgent()
			return a.Out.Emit(Row{
				Cols:  []string{"mode", "user_agent"},
				Vals:  []string{mode, ua},
				Value: map[string]any{"mode": mode, "user_agent": ua},
			})
		},
	}
}

func newConfigCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show resolved configuration and paths",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Print the effective configuration",
		RunE: func(cmd *cobra.Command, _ []string) error {
			defer func() { _ = a.Out.Flush() }()
			cfg := a.Cfg
			rows := [][2]string{
				{"mode", a.Client.Mode()},
				{"lang", cfg.Lang},
				{"delay", cfg.Delay.String()},
				{"retries", fmt.Sprint(cfg.Retries)},
				{"timeout", cfg.Timeout.String()},
				{"cache_dir", cfg.CacheDir},
				{"cache_ttl", cfg.CacheTTL.String()},
				{"data_dir", cfg.DataDir},
				{"no_cache", fmt.Sprint(cfg.NoCache)},
			}
			for _, r := range rows {
				if err := a.Out.Emit(Row{
					Cols:  []string{"key", "value"},
					Vals:  []string{r[0], r[1]},
					Value: map[string]string{"key": r[0], "value": r[1]},
				}); err != nil {
					return err
				}
			}
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "path",
		Short: "Print the config, cache, and data directories",
		Run: func(cmd *cobra.Command, _ []string) {
			cfgDir, _ := os.UserConfigDir()
			_, _ = fmt.Fprintln(os.Stdout, "config:", cfgDir)
			_, _ = fmt.Fprintln(os.Stdout, "cache: ", a.Cfg.CacheDir)
			_, _ = fmt.Fprintln(os.Stdout, "data:  ", a.Cfg.DataDir)
		},
	})
	return cmd
}

func newCacheCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cache",
		Short: "Inspect and clear the on-disk cache",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "dir",
		Short: "Print the cache directory",
		Run: func(cmd *cobra.Command, _ []string) {
			_, _ = fmt.Fprintln(os.Stdout, a.Client.Cache().Dir())
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "clear",
		Short: "Delete every cached response",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := a.Client.Cache().Clear(); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(os.Stderr, "[th] cache cleared:", a.Client.Cache().Dir())
			return nil
		},
	})
	return cmd
}
