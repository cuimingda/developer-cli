package cmd

import "github.com/spf13/cobra"

func newConfigCmd(initializer *ConfigInitializer) *cobra.Command {
	if initializer == nil {
		initializer = newDefaultConfigInitializer()
	}

	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage user configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(newConfigInitCmd(initializer))

	return cmd
}
