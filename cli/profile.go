package cli

import (
	"github.com/spf13/cobra"
)

func newProfileCmd(a *App) *cobra.Command {
	var posts, replies bool
	cmd := &cobra.Command{
		Use:   "profile <@handle|id|url>",
		Short: "A profile's full record, or its posts/replies",
		Long: `Resolve a profile to a rich record from the crawler-rendered page.

With --posts, walk the profile's recent posts (the server-rendered window, then
the logged-out GraphQL query while it is current). With --replies, walk the
profile's replies feed instead.`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			defer func() { _ = a.Out.Flush() }()
			ctx := cmd.Context()
			target := args[0]

			switch {
			case posts:
				a.progress("walking posts for %s", target)
				for p, err := range a.Client.ProfilePosts(ctx, target, a.Limit) {
					if err != nil {
						return err
					}
					if err := a.Out.Emit(postRow(&p)); err != nil {
						return err
					}
				}
				return nil
			case replies:
				a.progress("walking replies for %s", target)
				for p, err := range a.Client.ProfileReplies(ctx, target, a.Limit) {
					if err != nil {
						return err
					}
					if err := a.Out.Emit(postRow(&p)); err != nil {
						return err
					}
				}
				return nil
			default:
				prof, err := a.Client.Profile(ctx, target)
				if err != nil {
					return err
				}
				return a.Out.Emit(profileRow(prof))
			}
		},
	}
	cmd.Flags().BoolVar(&posts, "posts", false, "walk the profile's recent posts")
	cmd.Flags().BoolVar(&replies, "replies", false, "walk the profile's replies feed")
	return cmd
}
