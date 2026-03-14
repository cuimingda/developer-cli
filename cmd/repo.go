package cmd

import "github.com/spf13/cobra"

func newRepoCmd(initializer *ConfigInitializer) *cobra.Command {
	if initializer == nil {
		initializer = newDefaultConfigInitializer()
	}

	cmd := &cobra.Command{
		Use:   "repo",
		Short: "Manage accessible repositories",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(newRepoListCmd(newRepoListRunner(initializer)))

	return cmd
}
