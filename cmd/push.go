package cmd

import "github.com/spf13/cobra"

func newPushCmd(runner workspacePushCommandRunner) *cobra.Command {
	if runner == nil {
		runner = newDefaultWorkspacePushRunner(nil)
	}

	return &cobra.Command{
		Use:          "push",
		Short:        "Alias for workspace push",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner.Run(cmd.OutOrStdout())
		},
	}
}
