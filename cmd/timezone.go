package cmd

import "github.com/spf13/cobra"

func newTimezoneCmd(runner *TimezoneRunner) *cobra.Command {
	if runner == nil {
		runner = newTimezoneRunner()
	}

	return &cobra.Command{
		Use:          "timezone",
		Short:        "Show the local time zone at command execution time",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner.Run(cmd.OutOrStdout())
		},
	}
}
