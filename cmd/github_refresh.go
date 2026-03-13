package cmd

import "github.com/spf13/cobra"

func newGitHubRefreshCmd(runner *GitHubRefreshRunner) *cobra.Command {
	if runner == nil {
		runner = newGitHubRefreshRunner(nil)
	}

	return &cobra.Command{
		Use:          "refresh",
		Short:        "Refresh the GitHub user access token",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner.Run(cmd.Context(), cmd.OutOrStdout())
		},
	}
}
