package cli

import (
	"github.com/Egor01KKK/threads-content-research-agent/threads"
	"github.com/spf13/cobra"
)

func newDBCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Build and query a local SQLite dataset",
	}
	cmd.AddCommand(newDBBuildCmd(a), newDBQueryCmd(a))
	return cmd
}

func newDBBuildCmd(a *App) *cobra.Command {
	var dbPath string
	var replies, posts bool
	cmd := &cobra.Command{
		Use:   "build <@handle|id|url>",
		Short: "Crawl a profile's posts into a SQLite table",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			store, err := threads.OpenStore(dbPath)
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()

			n := 0
			seq := a.Client.ProfilePosts(ctx, args[0], a.Limit)
			if replies {
				seq = a.Client.ProfileReplies(ctx, args[0], a.Limit)
			}
			for p, err := range seq {
				if err != nil {
					return err
				}
				if err := store.PutPost(p); err != nil {
					return err
				}
				n++
			}
			a.progress("wrote %d posts to %s", n, dbPath)
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "threads.db", "SQLite dataset path")
	cmd.Flags().BoolVar(&posts, "posts", true, "crawl the profile's posts (the default)")
	cmd.Flags().BoolVar(&replies, "replies", false, "crawl the replies feed instead of posts")
	return cmd
}

func newDBQueryCmd(a *App) *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:   "query <sql>",
		Short: "Run SQL against the local dataset",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			defer func() { _ = a.Out.Flush() }()
			store, err := threads.OpenStore(dbPath)
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()

			cols, rows, err := store.Query(args[0])
			if err != nil {
				return err
			}
			for _, r := range rows {
				rec := map[string]string{}
				for i, c := range cols {
					if i < len(r) {
						rec[c] = r[i]
					}
				}
				if err := a.Out.Emit(Row{Cols: cols, Vals: r, Value: rec}); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "threads.db", "SQLite dataset path")
	return cmd
}
