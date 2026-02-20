package main

import (
	"fmt"
	"strings"
	"time"

	bcontext "github.com/jcadam/burrow/pkg/context"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(noteCmd)
}

var noteCmd = &cobra.Command{
	Use:   "note <text>",
	Short: "Add a note to the context ledger",
	Long:  "Appends a user note to the context ledger. Notes are included in context gathering and search, and are never pruned.",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		text := strings.Join(args, " ")

		ledger, err := openLedger()
		if err != nil {
			return err
		}

		err = ledger.Append(bcontext.Entry{
			Type:      bcontext.TypeNote,
			Label:     "Note",
			Timestamp: time.Now().UTC(),
			Content:   text,
		})
		if err != nil {
			return fmt.Errorf("adding note: %w", err)
		}

		fmt.Println("Note added to context ledger.")
		return nil
	},
}
