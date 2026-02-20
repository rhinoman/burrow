package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jcadam/burrow/pkg/config"
	"github.com/jcadam/burrow/pkg/contacts"
	bcontext "github.com/jcadam/burrow/pkg/context"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(contactsCmd)
	contactsCmd.AddCommand(contactsAddCmd)
	contactsCmd.AddCommand(contactsImportCmd)
	contactsCmd.AddCommand(contactsSearchCmd)
	contactsCmd.AddCommand(contactsShowCmd)
	contactsCmd.AddCommand(contactsRemoveCmd)
}

var contactsCmd = &cobra.Command{
	Use:   "contacts",
	Short: "Manage your contacts",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openContactStore()
		if err != nil {
			return err
		}

		list, err := store.List()
		if err != nil {
			return fmt.Errorf("listing contacts: %w", err)
		}

		if len(list) == 0 {
			fmt.Println("No contacts. Use 'gd contacts add' or 'gd contacts import <file>'.")
			return nil
		}

		for _, c := range list {
			line := c.Name
			if c.Email != "" {
				line += " — " + c.Email
			}
			if c.Organization != "" || c.Title != "" {
				parts := []string{}
				if c.Organization != "" {
					parts = append(parts, c.Organization)
				}
				if c.Title != "" {
					parts = append(parts, c.Title)
				}
				line += " — " + strings.Join(parts, ", ")
			}
			fmt.Printf("  %s\n", line)
		}
		fmt.Printf("\n%d contact(s)\n", len(list))
		return nil
	},
}

var contactsAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a contact interactively",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openContactStore()
		if err != nil {
			return err
		}

		scanner := bufio.NewScanner(os.Stdin)
		c := &contacts.Contact{}

		fmt.Print("Name (required): ")
		if !scanner.Scan() {
			return nil
		}
		c.Name = strings.TrimSpace(scanner.Text())
		if c.Name == "" {
			return fmt.Errorf("name is required")
		}

		fmt.Print("Email: ")
		if scanner.Scan() {
			c.Email = strings.TrimSpace(scanner.Text())
		}

		fmt.Print("Organization: ")
		if scanner.Scan() {
			c.Organization = strings.TrimSpace(scanner.Text())
		}

		fmt.Print("Title: ")
		if scanner.Scan() {
			c.Title = strings.TrimSpace(scanner.Text())
		}

		fmt.Print("Phone: ")
		if scanner.Scan() {
			c.Phone = strings.TrimSpace(scanner.Text())
		}

		fmt.Print("Tags (semicolon-separated): ")
		if scanner.Scan() {
			raw := strings.TrimSpace(scanner.Text())
			if raw != "" {
				for _, t := range strings.Split(raw, ";") {
					t = strings.TrimSpace(t)
					if t != "" {
						c.Tags = append(c.Tags, t)
					}
				}
			}
		}

		fmt.Print("Notes: ")
		if scanner.Scan() {
			c.Notes = strings.TrimSpace(scanner.Text())
		}

		if err := store.Add(c); err != nil {
			return fmt.Errorf("adding contact: %w", err)
		}

		fmt.Printf("Added %s\n", c.Name)

		// Index in ledger
		indexContactInLedger(openLedgerForContacts(), c)
		return nil
	},
}

var contactsImportCmd = &cobra.Command{
	Use:   "import <file>",
	Short: "Import contacts from CSV or vCard file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := args[0]

		store, err := openContactStore()
		if err != nil {
			return err
		}

		// Parse first, then add — so we can track exactly which contacts
		// were imported (for ledger indexing and accurate count).
		var parsed []*contacts.Contact
		var warnings []string

		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".csv":
			parsed, warnings, err = contacts.ParseCSV(path)
		case ".vcf":
			var data []byte
			data, err = os.ReadFile(path)
			if err == nil {
				parsed, warnings, err = contacts.ParseVCard(data)
			}
		default:
			return fmt.Errorf("unsupported file format %q (use .csv or .vcf)", ext)
		}

		if err != nil {
			return fmt.Errorf("importing contacts: %w", err)
		}

		added := 0
		for _, c := range parsed {
			if err := store.Add(c); err != nil {
				warnings = append(warnings, fmt.Sprintf("skipping %q: %v", c.Name, err))
				continue
			}
			added++
		}

		for _, w := range warnings {
			fmt.Fprintf(os.Stderr, "  warning: %s\n", w)
		}

		fmt.Printf("Imported %d contact(s)\n", added)

		// Index only newly imported contacts in ledger (one ledger instance)
		ledger := openLedgerForContacts()
		for _, c := range parsed {
			indexContactInLedger(ledger, c)
		}
		return nil
	},
}

var contactsSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search contacts",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := strings.Join(args, " ")

		store, err := openContactStore()
		if err != nil {
			return err
		}

		results, err := store.Search(query)
		if err != nil {
			return fmt.Errorf("searching contacts: %w", err)
		}

		if len(results) == 0 {
			fmt.Printf("No contacts matching %q\n", query)
			return nil
		}

		fmt.Printf("Found %d contact(s):\n\n", len(results))
		for _, c := range results {
			line := c.Name
			if c.Email != "" {
				line += " — " + c.Email
			}
			if c.Organization != "" {
				line += " — " + c.Organization
			}
			fmt.Printf("  %s\n", line)
		}
		return nil
	},
}

var contactsShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show a contact's details",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := strings.Join(args, " ")

		store, err := openContactStore()
		if err != nil {
			return err
		}

		// Try lookup (fuzzy) first, then exact slug
		c := store.Lookup(name)
		if c == nil {
			c, err = store.Get(name)
			if err != nil {
				return fmt.Errorf("contact %q not found", name)
			}
		}

		fmt.Printf("\n  Name:         %s\n", c.Name)
		if c.Email != "" {
			fmt.Printf("  Email:        %s\n", c.Email)
		}
		if c.Organization != "" {
			fmt.Printf("  Organization: %s\n", c.Organization)
		}
		if c.Title != "" {
			fmt.Printf("  Title:        %s\n", c.Title)
		}
		if c.Phone != "" {
			fmt.Printf("  Phone:        %s\n", c.Phone)
		}
		if len(c.Tags) > 0 {
			fmt.Printf("  Tags:         %s\n", strings.Join(c.Tags, ", "))
		}
		if c.Notes != "" {
			fmt.Printf("  Notes:        %s\n", c.Notes)
		}
		if c.Added != "" {
			fmt.Printf("  Added:        %s\n", c.Added)
		}
		fmt.Println()
		return nil
	},
}

var contactsRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a contact",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := strings.Join(args, " ")

		store, err := openContactStore()
		if err != nil {
			return err
		}

		// Resolve name to slug via Lookup
		c := store.Lookup(name)
		if c == nil {
			// Try as literal slug
			if err := store.Remove(name); err != nil {
				return fmt.Errorf("contact %q not found", name)
			}
			fmt.Printf("Removed %s\n", name)
			return nil
		}

		slugName := contacts.SlugFor(c.Name)
		if err := store.Remove(slugName); err != nil {
			return fmt.Errorf("removing contact: %w", err)
		}

		fmt.Printf("Removed %s\n", c.Name)
		return nil
	},
}

// openContactStore opens the contacts store from the standard location.
func openContactStore() (*contacts.Store, error) {
	burrowDir, err := config.BurrowDir()
	if err != nil {
		return nil, err
	}
	contactsDir := filepath.Join(burrowDir, "contacts")
	return contacts.NewStore(contactsDir)
}

// openLedgerForContacts opens the context ledger from the standard location.
// Returns nil if the ledger can't be opened (non-fatal).
func openLedgerForContacts() *bcontext.Ledger {
	burrowDir, err := config.BurrowDir()
	if err != nil {
		return nil
	}
	contextDir := filepath.Join(burrowDir, "context")
	ledger, err := bcontext.NewLedger(contextDir)
	if err != nil {
		return nil
	}
	return ledger
}

// indexContactInLedger appends a contact entry to the context ledger.
func indexContactInLedger(ledger *bcontext.Ledger, c *contacts.Contact) {
	if ledger == nil {
		return
	}

	content := c.Name
	if c.Email != "" {
		content += " <" + c.Email + ">"
	}
	if c.Organization != "" {
		content += " — " + c.Organization
	}
	if c.Title != "" {
		content += ", " + c.Title
	}

	ledger.Append(bcontext.Entry{
		Type:      bcontext.TypeContact,
		Label:     "Contact: " + c.Name,
		Timestamp: time.Now().UTC(),
		Content:   content,
	})
}
