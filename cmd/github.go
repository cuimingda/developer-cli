package cmd

import "github.com/spf13/cobra"

func newGitHubCmd(initializer *ConfigInitializer, runner *GitHubLoginRunner) *cobra.Command {
	if initializer == nil {
		initializer = newDefaultConfigInitializer()
	}
	if runner == nil {
		runner = newGitHubLoginRunner(initializer)
	}

	cmd := &cobra.Command{
		Use:   "github",
		Short: "Manage GitHub authentication",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(newGitHubLoginCmd(runner))
	cmd.AddCommand(newGitHubLogoutCmd(newGitHubLogoutRunner(initializer)))
	cmd.AddCommand(newGitHubRefreshCmd(newGitHubRefreshRunner(initializer)))
	cmd.AddCommand(newGitHubStatusCmd(newGitHubAuthStatusRunner(initializer)))

	return cmd
}
