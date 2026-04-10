package main

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/mistakeknot/Ockham/internal/costgate"
	"github.com/mistakeknot/Ockham/internal/notify"
	"github.com/spf13/cobra"
)

var notifyDigest bool
var notifyPending bool
var notifyTest bool
var notifyJSON bool

var notifyCmd = &cobra.Command{
	Use:   "notify",
	Short: "Manage notification outbox records",
	RunE:  runNotify,
}

func init() {
	notifyCmd.Flags().BoolVar(&notifyDigest, "digest", false, "Enqueue a digest notification")
	notifyCmd.Flags().BoolVar(&notifyPending, "pending", false, "Show pending outbox notifications")
	notifyCmd.Flags().BoolVar(&notifyTest, "test", false, "Enqueue a test alert notification")
	notifyCmd.Flags().BoolVar(&notifyJSON, "json", false, "Output as JSON")
	rootCmd.AddCommand(notifyCmd)
}

func runNotify(cmd *cobra.Command, args []string) error {
	outbox, err := notify.NewOutbox(notify.DefaultOutboxPath())
	if err != nil {
		return err
	}
	defer outbox.Close()

	now := time.Now()
	switch {
	case notifyTest:
		n := notify.NewAlert("Ockham test alert", "This is a test notification for Hermes delivery.", 1, now)
		if err := outbox.Enqueue(n); err != nil {
			return err
		}
		fmt.Printf("Queued test alert: %s\n", n.ID)
		return nil

	case notifyDigest:
		body := buildDigestBody()
		n := notify.NewDigest("Ockham Digest", body, now)
		if err := outbox.Enqueue(n); err != nil {
			return err
		}
		fmt.Printf("Queued digest: %s\n", n.ID)
		return nil

	case notifyPending || (!notifyDigest && !notifyTest):
		pending, err := outbox.ListPending()
		if err != nil {
			return err
		}
		if notifyJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(pending)
		}
		if len(pending) == 0 {
			fmt.Println("No pending outbox notifications.")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "TYPE\tPRIORITY\tTITLE\tID")
		for _, n := range pending {
			fmt.Fprintf(w, "%s\t%d\t%s\t%s\n", n.Type, n.Priority, truncate(n.Title, 40), n.ID)
		}
		return w.Flush()
	}
	return nil
}

func buildDigestBody() string {
	db, err := costgate.NewApprovalDB(costgate.DefaultDBPath())
	if err != nil {
		return "Daily digest unavailable: could not open approvals database."
	}
	defer db.Close()
	gate := costgate.NewGate(nil, db)
	pending, err := gate.Pending()
	if err != nil {
		return "Daily digest unavailable: could not read pending approvals."
	}
	return fmt.Sprintf("Pending approvals: %d\nUse `ockham cost pending` for details.", len(pending))
}
