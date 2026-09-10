package cli

import (
	"github.com/Egor01KKK/threads-content-research-agent/pkg/thid"
	"github.com/spf13/cobra"
)

func newPostCmd(a *App) *cobra.Command {
	var replies, raw bool
	cmd := &cobra.Command{
		Use:   "post <url|shortcode|id>",
		Short: "A single post in full, or its reply thread",
		Long: `Fetch a single post from its crawler-rendered permalink.

The first parsed post is the subject; the rest of the page is its top replies.
With --replies, stream those replies (extended by the logged-out post-page
GraphQL query). With --raw, print the upstream HTML untouched.`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			defer func() { _ = a.Out.Flush() }()
			ctx := cmd.Context()
			input := args[0]

			if raw {
				target := thid.Classify(input).URL
				if target == "" {
					target = input
				}
				b, err := a.Client.GetRaw(ctx, target)
				if err != nil {
					return err
				}
				return a.Out.Raw(b)
			}
			if replies {
				a.progress("walking replies for %s", input)
				for r, err := range a.Client.PostReplies(ctx, input, a.Limit) {
					if err != nil {
						return err
					}
					if err := a.Out.Emit(replyRow(&r)); err != nil {
						return err
					}
				}
				return nil
			}
			p, err := a.Client.Post(ctx, input)
			if err != nil {
				return err
			}
			return a.Out.Emit(postRow(p))
		},
	}
	cmd.Flags().BoolVar(&replies, "replies", false, "stream the post's reply thread")
	cmd.Flags().BoolVar(&raw, "raw", false, "print the upstream HTML untouched")
	return cmd
}

func newRepliesCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "replies <url|shortcode|id>",
		Short: "Stream replies to a post as their own records",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			defer func() { _ = a.Out.Flush() }()
			ctx := cmd.Context()
			for r, err := range a.Client.PostReplies(ctx, args[0], a.Limit) {
				if err != nil {
					return err
				}
				if err := a.Out.Emit(replyRow(&r)); err != nil {
					return err
				}
			}
			return nil
		},
	}
}
