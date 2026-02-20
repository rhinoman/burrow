package main

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:   "gd",
	Short: "Burrow — personal research assistant",
	Long:  "Burrow queries services on a schedule, synthesizes results, and produces actionable reports. It never acts on your behalf.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInteractive(cmd.Context())
	},
}
