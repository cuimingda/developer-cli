package cmd

import (
	"io"

	"github.com/spf13/cobra"
)

type workspacePushCommandRunner interface {
	Run(stdout io.Writer) error
}

func newWorkspacePushCmd(runner workspacePushCommandRunner) *cobra.Command {
	if runner == nil {
		runner = newDefaultWorkspacePushRunner(nil)
	}

	return &cobra.Command{
		Use:          "push",
		Short:        "Push clean, unsynced workspace projects",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner.Run(cmd.OutOrStdout())
		},
	}
}
