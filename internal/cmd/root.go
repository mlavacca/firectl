package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/mlavacca/firectl/internal/config"
	"github.com/mlavacca/firectl/internal/processor"
)

var (
	dryRun   bool
	provider string

	rootCmd = &cobra.Command{
		Use:   "firectl [csv-file]",
		Short: "Import transactions from CSV into Firefly III",
		Long: `A CLI tool to import transactions from CSV files into Firefly III.

The tool reads a CSV file containing transactions and imports them into your
Firefly III instance. It automatically detects headers, maps accounts and
categories, and prevents duplicate transactions using external IDs.

Configuration:
  - FIREFLY_URL and FIREFLY_TOKEN environment variables (or .env file)

Providers:
  Use --provider to specify which bank format to use:
  - satispay: Satispay transaction exports
  - sanpaolo: Intesa Sanpaolo bank statements

Example:
  firectl --provider satispay transactions.csv
  firectl --provider sanpaolo --dry-run statements.csv`,
		Args: cobra.ExactArgs(1),
		RunE: runImport,
	}
)

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Parse and validate without creating transactions")
	rootCmd.Flags().StringVar(&provider, "provider", "", "Bank provider (required: satispay, sanpaolo)")
	_ = rootCmd.MarkFlagRequired("provider")
}

func runImport(cmd *cobra.Command, args []string) error {
	csvFile := args[0]

	// Check if file exists
	if _, err := os.Stat(csvFile); os.IsNotExist(err) {
		return fmt.Errorf("file not found: %s", csvFile)
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Create processor
	proc := processor.NewProcessor(cfg)

	// Process transactions
	fmt.Fprintf(os.Stderr, "Processing: %s (provider: %s)\n", csvFile, provider)
	if dryRun {
		fmt.Fprintf(os.Stderr, "Running in dry-run mode (no transactions will be created)\n")
	}

	results, err := proc.Process(csvFile, provider, dryRun)
	if err != nil {
		return err
	}

	// Print summary
	printSummary(results)

	return nil
}

func printSummary(results []processor.Result) {
	created := 0
	skipped := 0
	errors := 0
	var errorMsgs []string

	for _, result := range results {
		switch result.Status {
		case "created":
			created++
		case "dry-run":
			fmt.Printf("Row %d: Would be created\n", result.Row)
			if result.Transaction != nil {
				fmt.Printf("  Date: %s\n", result.Transaction.Date)
				fmt.Printf("  Description: %s\n", result.Transaction.Description)
				fmt.Printf("  Amount: %s\n", result.Transaction.Amount)
				if result.Transaction.Type != nil {
					fmt.Printf("  Type: %s\n", *result.Transaction.Type)
				}
				if result.Transaction.Source != nil {
					fmt.Printf("  Source: %s\n", *result.Transaction.Source)
				}
				if result.Transaction.Destination != nil {
					fmt.Printf("  Destination: %s\n", *result.Transaction.Destination)
				}
				if result.Transaction.Category != nil {
					fmt.Printf("  Category: %s\n", *result.Transaction.Category)
				}
				if result.Transaction.Budget != nil {
					fmt.Printf("  Budget: %s\n", *result.Transaction.Budget)
				}
				if result.Transaction.Notes != nil && *result.Transaction.Notes != "" {
					fmt.Printf("  Notes: %s\n", *result.Transaction.Notes)
				}
				if len(result.Transaction.Tags) > 0 {
					fmt.Printf("  Tags: %v\n", result.Transaction.Tags)
				}
				fmt.Println()
			}
		case "error":
			errors++
			errorMsgs = append(errorMsgs, fmt.Sprintf("Row %d: %v", result.Row, result.Error))
		default:
			if result.Status != "" {
				skipped++
			}
		}
	}

	fmt.Println()
	fmt.Printf("Processed %d rows\n", len(results))
	fmt.Printf("Created: %d\n", created)
	fmt.Printf("Skipped: %d\n", skipped)
	fmt.Printf("Errors: %d\n", errors)

	if len(errorMsgs) > 0 {
		fmt.Println("\nErrors:")
		for _, msg := range errorMsgs {
			fmt.Println(msg)
		}
	}
}
