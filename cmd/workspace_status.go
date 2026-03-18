package cmd

import (
	"io"

	"github.com/spf13/cobra"
)

type workspaceStatusCommandRunner interface {
	Run(stdout io.Writer) error
}

func newWorkspaceStatusCmd(runner workspaceStatusCommandRunner) *cobra.Command {
	if runner == nil {
		runner = newDefaultWorkspaceStatusRunner(nil)
	}

	return &cobra.Command{
		Use:          "status",
		Short:        "Show Git status for each workspace project",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner.Run(cmd.OutOrStdout())
		},
	}
}
