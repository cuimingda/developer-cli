package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newConfigInitCmd(initializer *ConfigInitializer) *cobra.Command {
	if initializer == nil {
		initializer = newDefaultConfigInitializer()
	}

	return &cobra.Command{
		Use:   "init",
		Short: "Initialize the user config file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, err := initializer.Init()
			if err != nil {
				return err
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "initialized config at %s\n", configPath)
			return err
		},
	}
}
