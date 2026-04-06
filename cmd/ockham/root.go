package main

import (
	"strings"

	"github.com/mistakeknot/Ockham/internal/halt"
	"github.com/spf13/cobra"
)

// haltAllowed lists commands that may run when factory is halted.
// check MUST be here — runCheck() is designed to work correctly when halted
// (reconstructs halt, snapshots authority, skips evaluation).
var haltAllowed = map[string]bool{
	"check": true, "health": true, "signals": true, "resume": true,
	"intent show": true, "intent validate": true,
	"help": true, "version": true,
}

var rootCmd = &cobra.Command{
	Use:   "ockham",
	Short: "Factory governor — translates strategic intent to dispatch weights",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		name := cmd.CommandPath()
		name = strings.TrimPrefix(name, "ockham ")
		if haltAllowed[name] {
			return nil
		}
		h := halt.New(halt.DefaultSentinelPath())
		return h.RequireRunning()
	},
}
