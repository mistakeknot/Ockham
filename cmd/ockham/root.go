package main

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:   "ockham",
	Short: "Factory governor — translates strategic intent to dispatch weights",
}
