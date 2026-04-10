package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/mistakeknot/Ockham/internal/config"
	"github.com/mistakeknot/Ockham/internal/costgate"
	"github.com/spf13/cobra"
)

var costApproveTier int
var costCheckAgent string
var costJSON bool
var costApprovedBy string

var costCmd = &cobra.Command{
	Use:   "cost",
	Short: "Manage cost tiers and approvals",
}

var costShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Display cost tier definitions",
	RunE:  runCostShow,
}

var costCheckCmd = &cobra.Command{
	Use:   "check <bead-id>",
	Short: "Check whether a bead can be dispatched to an agent",
	Args:  cobra.ExactArgs(1),
	RunE:  runCostCheck,
}

var costApproveCmd = &cobra.Command{
	Use:   "approve <bead-id>",
	Short: "Approve a bead for a given cost tier ceiling",
	Args:  cobra.ExactArgs(1),
	RunE:  runCostApprove,
}

var costPendingCmd = &cobra.Command{
	Use:   "pending",
	Short: "Show pending approval requests",
	RunE:  runCostPending,
}

func init() {
	costShowCmd.Flags().BoolVar(&costJSON, "json", false, "Output as JSON")
	costCheckCmd.Flags().BoolVar(&costJSON, "json", false, "Output as JSON")
	costPendingCmd.Flags().BoolVar(&costJSON, "json", false, "Output as JSON")

	costCheckCmd.Flags().StringVar(&costCheckAgent, "agent", "", "Agent name to evaluate")
	costApproveCmd.Flags().IntVar(&costApproveTier, "tier", -1, "Maximum approved cost tier")
	costApproveCmd.Flags().StringVar(&costApprovedBy, "by", "principal", "Approver identity")

	costCmd.AddCommand(costShowCmd)
	costCmd.AddCommand(costCheckCmd)
	costCmd.AddCommand(costApproveCmd)
	costCmd.AddCommand(costPendingCmd)
	rootCmd.AddCommand(costCmd)
}

func openGate() (*costgate.Gate, *costgate.ApprovalDB, error) {
	store := config.NewStore(config.DefaultStorePath())
	db, err := costgate.NewApprovalDB(costgate.DefaultDBPath())
	if err != nil {
		return nil, nil, err
	}
	return costgate.NewGate(store, db), db, nil
}

func runCostShow(cmd *cobra.Command, args []string) error {
	store := config.NewStore(config.DefaultStorePath())
	cfg, err := store.Load()
	if err != nil {
		return err
	}
	if costJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(cfg.Cost)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "TIER\tNAME\tAUTO\tREQUIRES\tDESCRIPTION")
	for tier := 0; tier <= 3; tier++ {
		def := cfg.Cost.Tiers[tier]
		auto := "no"
		if def.AutoApprove {
			auto = "yes"
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", tier, def.Name, auto, def.Requires, def.Description)
	}
	return w.Flush()
}

func runCostCheck(cmd *cobra.Command, args []string) error {
	if costCheckAgent == "" {
		return fmt.Errorf("--agent is required")
	}
	gate, db, err := openGate()
	if err != nil {
		return err
	}
	defer db.Close()
	d, err := gate.Evaluate(args[0], costCheckAgent)
	if err != nil {
		return err
	}
	if costJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(d)
	}
	fmt.Printf("Bead: %s\nAgent: %s\nTier: %d\nStatus: %s\n", d.BeadID, d.Agent, d.Tier, d.Status)
	if d.Reason != "" {
		fmt.Printf("Reason: %s\n", d.Reason)
	}
	if d.ApprovedBy != "" {
		fmt.Printf("Approved by: %s\n", d.ApprovedBy)
	}
	return nil
}

func runCostApprove(cmd *cobra.Command, args []string) error {
	if costApproveTier < 0 || costApproveTier > 3 {
		return fmt.Errorf("--tier must be in [0,3]")
	}
	gate, db, err := openGate()
	if err != nil {
		return err
	}
	defer db.Close()
	if err := gate.Approve(args[0], costApproveTier, costApprovedBy); err != nil {
		return err
	}
	fmt.Printf("Approved %s up to tier %d\n", args[0], costApproveTier)
	return nil
}

func runCostPending(cmd *cobra.Command, args []string) error {
	gate, db, err := openGate()
	if err != nil {
		return err
	}
	defer db.Close()
	pending, err := gate.Pending()
	if err != nil {
		return err
	}
	if costJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(pending)
	}
	if len(pending) == 0 {
		fmt.Println("No pending approval requests.")
		return nil
	}
	sort.SliceStable(pending, func(i, j int) bool {
		if pending[i].RequestedAt != pending[j].RequestedAt {
			return pending[i].RequestedAt < pending[j].RequestedAt
		}
		return pending[i].BeadID < pending[j].BeadID
	})
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "BEAD\tAGENT\tTIER\tREASON")
	for _, d := range pending {
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", d.BeadID, d.Agent, d.Tier, d.Reason)
	}
	return w.Flush()
}
