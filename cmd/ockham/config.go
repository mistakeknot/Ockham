package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/mistakeknot/Ockham/internal/config"
	"github.com/spf13/cobra"
)

var configJSON bool

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage Ockham configuration",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Display current configuration",
	RunE:  runConfigShow,
}

var configValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate configuration file",
	RunE:  runConfigValidate,
}

func init() {
	configShowCmd.Flags().BoolVar(&configJSON, "json", false, "Output as JSON")

	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configValidateCmd)
	rootCmd.AddCommand(configCmd)
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	store := config.NewStore(config.DefaultStorePath())
	f, err := store.Load()
	if err != nil {
		return err
	}

	if configJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(f)
	}

	// Orgs
	fmt.Println("ORGS:")
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "  NAME\tSAML")
	for _, org := range f.Orgs {
		saml := ""
		if org.SAML {
			saml = "yes"
		}
		fmt.Fprintf(w, "  %s\t%s\n", org.Name, saml)
	}
	w.Flush()

	// Agents
	fmt.Println("\nAGENTS:")
	w = tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "  NAME\tMODEL\tTIER\tRUNTIME")
	names := make([]string, 0, len(f.Agents))
	for name := range f.Agents {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		a := f.Agents[name]
		fmt.Fprintf(w, "  %s\t%s\t%d\t%s\n", name, a.Model, a.CostTier, a.Runtime)
	}
	w.Flush()

	// Cost tiers
	fmt.Println("\nCOST TIERS:")
	w = tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "  TIER\tNAME\tAUTO\tDESCRIPTION")
	for tier := 0; tier <= 3; tier++ {
		if def, ok := f.Cost.Tiers[tier]; ok {
			auto := "no"
			if def.AutoApprove {
				auto = "yes"
			}
			fmt.Fprintf(w, "  %d\t%s\t%s\t%s\n", tier, def.Name, auto, def.Description)
		}
	}
	w.Flush()

	fmt.Printf("\nSource: %s\n", store.Path())
	return nil
}

func runConfigValidate(cmd *cobra.Command, args []string) error {
	store := config.NewStore(config.DefaultStorePath())
	f, err := store.Load()
	if err != nil {
		return err
	}

	if err := config.Validate(f); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
		return fmt.Errorf("validation failed")
	}

	fmt.Println("Config file valid")
	return nil
}
