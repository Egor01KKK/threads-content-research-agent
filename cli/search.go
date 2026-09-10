package cli

import (
	"strings"

	"github.com/Egor01KKK/threads-content-research-agent/threads"
	"github.com/spf13/cobra"
)

func newSearchCmd(a *App) *cobra.Command {
	var typ string
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Keyword search across public posts",
		Long: `Search the public server-rendered Threads search page for a keyword.

The rotating logged-out GraphQL query remains a compatibility fallback when the
page does not carry embedded results.`,
		Args: minArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			defer func() { _ = a.Out.Flush() }()
			if normalized := strings.ToLower(strings.TrimSpace(typ)); normalized != "" && normalized != "top" {
				return threads.Usagef("unsupported search type %q; only top search is currently supported", typ)
			}
			ctx := cmd.Context()
			query := strings.Join(args, " ")
			a.progress("searching %q", query)
			for r, err := range a.Client.Search(ctx, query, a.Limit) {
				if err != nil {
					return err
				}
				if err := a.Out.Emit(searchRow(&r)); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&typ, "type", "top", "search ordering (top; recent is not currently supported)")
	return cmd
}
