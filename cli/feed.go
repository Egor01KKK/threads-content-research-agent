package cli

import (
	"github.com/spf13/cobra"
)

func newFeedCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "feed <@handle|id|url>",
		Short: "Walk a profile's most-recent posts (alias for profile --posts)",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			defer func() { _ = a.Out.Flush() }()
			ctx := cmd.Context()
			a.progress("walking feed for %s", args[0])
			for p, err := range a.Client.ProfilePosts(ctx, args[0], a.Limit) {
				if err != nil {
					return err
				}
				if err := a.Out.Emit(postRow(&p)); err != nil {
					return err
				}
			}
			return nil
		},
	}
}
