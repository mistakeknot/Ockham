package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/mistakeknot/Ockham/internal/intent"
	"github.com/spf13/cobra"
)

var (
	intentTheme    string
	intentBudget   float64
	intentPriority string
	intentFreeze   string
)

var intentCmd = &cobra.Command{
	Use:   "intent",
	Short: "Manage theme budgets and priorities",
	RunE:  runIntentSet,
}

var intentShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Display current intent directives",
	RunE:  runIntentShow,
}

var intentValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate the intent file",
	RunE:  runIntentValidate,
}

func init() {
	intentCmd.Flags().StringVar(&intentTheme, "theme", "", "Theme name")
	intentCmd.Flags().Float64Var(&intentBudget, "budget", -1, "Budget fraction (0-1)")
	intentCmd.Flags().StringVar(&intentPriority, "priority", "normal", "Priority (high|normal|low)")
	intentCmd.Flags().StringVar(&intentFreeze, "freeze", "", "Freeze a theme (add to constraints)")

	intentCmd.AddCommand(intentShowCmd)
	intentCmd.AddCommand(intentValidateCmd)
	rootCmd.AddCommand(intentCmd)
}

func runIntentSet(cmd *cobra.Command, args []string) error {
	// Halt guard handled by PersistentPreRunE allowlist in root.go

	store := intent.NewStore(intent.DefaultStorePath())

	// Handle --freeze
	if intentFreeze != "" {
		f, err := store.Load()
		if err != nil {
			return err
		}
		for _, existing := range f.Constraints.Freeze {
			if existing == intentFreeze {
				fmt.Fprintf(os.Stderr, "theme %q already frozen\n", intentFreeze)
				return nil
			}
		}
		f.Constraints.Freeze = append(f.Constraints.Freeze, intentFreeze)
		if err := intent.Validate(f); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return fmt.Errorf("validation failed")
		}
		if err := store.Save(f); err != nil {
			return err
		}
		fmt.Printf("Frozen theme %q\n", intentFreeze)
		return nil
	}

	// Handle --theme + --budget/--priority
	if intentTheme == "" {
		return cmd.Help()
	}

	f, err := store.Load()
	if err != nil {
		return err
	}
	if f.Themes == nil {
		f.Themes = make(map[string]intent.ThemeBudget)
	}

	tb := f.Themes[intentTheme]

	if intentBudget >= 0 {
		tb.Budget = intentBudget
	}

	if cmd.Flags().Changed("priority") {
		p, err := intent.ParsePriority(intentPriority)
		if err != nil {
			return err
		}
		tb.Priority = p
	} else if tb.Priority == "" {
		tb.Priority = intent.PriorityNormal
	}

	f.Themes[intentTheme] = tb
	f.Version = 1

	if err := intent.Validate(f); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return fmt.Errorf("validation failed — file not saved")
	}

	if err := store.Save(f); err != nil {
		return err
	}

	fmt.Printf("Updated theme %q: budget=%.2f priority=%s\n", intentTheme, tb.Budget, tb.Priority)
	return nil
}

func runIntentShow(cmd *cobra.Command, args []string) error {
	store := intent.NewStore(intent.DefaultStorePath())
	f, err := store.Load()
	if err != nil {
		return err
	}

	names := make([]string, 0, len(f.Themes))
	for name := range f.Themes {
		names = append(names, name)
	}
	sort.Strings(names)

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "THEME\tBUDGET\tPRIORITY\tOFFSET")
	for _, name := range names {
		tb := f.Themes[name]
		fmt.Fprintf(w, "%s\t%.0f%%\t%s\t%+d\n", name, tb.Budget*100, tb.Priority, tb.Priority.Offset())
	}
	w.Flush()

	if len(f.Constraints.Freeze) > 0 {
		fmt.Printf("\nFrozen: %s\n", strings.Join(f.Constraints.Freeze, ", "))
	}
	if len(f.Constraints.Focus) > 0 {
		fmt.Printf("Focus:  %s\n", strings.Join(f.Constraints.Focus, ", "))
	}

	return nil
}

func runIntentValidate(cmd *cobra.Command, args []string) error {
	store := intent.NewStore(intent.DefaultStorePath())
	f, err := store.Load()
	if err != nil {
		return err
	}

	if err := intent.Validate(f); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
		return fmt.Errorf("validation failed")
	}

	fmt.Println("Intent file valid")
	return nil
}
