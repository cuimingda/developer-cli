package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newWorkspaceListCmd(lister *WorkspaceLister) *cobra.Command {
	if lister == nil {
		lister = newDefaultWorkspaceLister(nil)
	}

	return &cobra.Command{
		Use:          "list",
		Short:        "List workspaces under the workspace root",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkspaceList(cmd, lister)
		},
	}
}

func runWorkspaceList(cmd *cobra.Command, lister *WorkspaceLister) error {
	entries, err := lister.List()
	if err != nil {
		return err
	}

	for _, entry := range entries {
		displayRemotePath := entry.RemotePath
		if !entry.HasRemote {
			displayRemotePath = redNoRemote
		}

		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s - %s\n", entry.LocalName, displayRemotePath); err != nil {
			return err
		}
	}

	return nil
}
