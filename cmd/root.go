/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var currentGOOS = func() string {
	return runtime.GOOS
}

func newRootCmd() *cobra.Command {
	return newRootCmdWithConfigInitializer(newDefaultConfigInitializer())
}

func newRootCmdWithConfigInitializer(initializer *ConfigInitializer) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "dev",
		Short:         "A brief description of your application",
		SilenceErrors: true,
		Long: `A longer description that spans multiple lines and likely contains
examples and usage of using your application. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	workspaceLister := newDefaultWorkspaceLister(initializer)
	cmd.AddCommand(newConfigCmd(initializer))
	cmd.AddCommand(newGitHubCmd(initializer, nil))
	cmd.AddCommand(newRepoCmd(initializer))
	cmd.AddCommand(newTimezoneCmd(nil))
	cmd.AddCommand(newWorkspaceCmd(initializer))
	cmd.AddCommand(newLSCmd(workspaceLister))

	return cmd
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = newRootCmd()

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() error {
	if currentGOOS() != "darwin" {
		return fmt.Errorf("dev only supports macOS")
	}

	return rootCmd.Execute()
}
