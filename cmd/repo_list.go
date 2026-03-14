package cmd

import (
	"context"
	"io"

	"github.com/spf13/cobra"
)

type repoListCommandRunner interface {
	Run(ctx context.Context, stdout io.Writer) error
}

func newRepoListCmd(runner repoListCommandRunner) *cobra.Command {
	if runner == nil {
		runner = newRepoListRunner(nil)
	}

	return &cobra.Command{
		Use:           "list",
		Short:         "List accessible GitHub repositories and local clone status",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner.Run(cmd.Context(), cmd.OutOrStdout())
		},
	}
}
