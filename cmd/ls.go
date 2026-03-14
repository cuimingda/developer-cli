package cmd

import "github.com/spf13/cobra"

func newLSCmd(lister *WorkspaceLister) *cobra.Command {
	if lister == nil {
		lister = newDefaultWorkspaceLister(nil)
	}

	return &cobra.Command{
		Use:          "ls",
		Short:        "Alias for workspace list",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkspaceList(cmd, lister)
		},
	}
}
