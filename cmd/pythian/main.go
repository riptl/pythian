package main

import (
	"github.com/spf13/cobra"
	"go.blockdaemon.com/pythian/cmd"
	"go.uber.org/zap"
)

var rootCmd = cobra.Command{
	Use:   "pythian",
	Short: "Research implementation of the Pyth Network client",

	PersistentPreRun: func(_ *cobra.Command, _ []string) {
		log = cmd.GetLogger()
	},
	CompletionOptions: cobra.CompletionOptions{
		HiddenDefaultCmd: true,
	},
}

func init() {
	rootCmd.PersistentFlags().AddFlagSet(cmd.FlagSetCommon)
}

func main() {
	cobra.CheckErr(rootCmd.Execute())
}

var log *zap.Logger
