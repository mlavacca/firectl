package processor

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/mlavacca/firectl/internal/config"
	"github.com/mlavacca/firectl/internal/firefly"
	"github.com/mlavacca/firectl/internal/parser"
	"github.com/mlavacca/firectl/internal/types"
)

// Result represents the result of processing a single transaction
type Result struct {
	Row         int
	Status      string
	ID          string
	Error       error
	Transaction *types.Transaction
}

// Processor handles transaction processing
type Processor struct {
	cfg    *config.Config
	client *firefly.Client
}

// NewProcessor creates a new transaction processor
func NewProcessor(cfg *config.Config) *Processor {
	return &Processor{
		cfg:    cfg,
		client: firefly.NewClient(cfg.FireflyURL, cfg.FireflyToken),
	}
}

// Process processes transactions from a CSV file
func (p *Processor) Process(filePath, providerName string, dryRun bool) ([]Result, error) {
	// Parse CSV file
	transactions, err := parser.ParseCSV(filePath, providerName)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CSV: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Found %d transactions to process\n", len(transactions))

	var results []Result

	for i := range transactions {
		rowNum := i + 1
		result := p.processTransaction(&transactions[i], rowNum, dryRun)
		results = append(results, result)
	}

	return results, nil
}

func (p *Processor) processTransaction(trans *types.Transaction, rowNum int, dryRun bool) Result {
	// Use transaction values as-is
	source := trans.Source
	destination := trans.Destination

	// Determine transaction type
	transType := ""
	if trans.Type != nil {
		transType = strings.ToLower(*trans.Type)
	} else {
		// Default to withdrawal if not specified
		transType = "withdrawal"
	}

	// Build notes string for external ID
	budget := trans.Budget
	notes := ""
	if trans.Notes != nil {
		notes = *trans.Notes
	}

	// Generate external ID for deduplication
	externalID := firefly.GenerateExternalID(
		trans.Date,
		trans.Amount,
		trans.Description,
		stringOrEmpty(source),
		stringOrEmpty(destination),
		notes,
	)

	// Build transaction data
	transData := firefly.TransactionData{
		Type:            transType,
		Description:     trans.Description,
		Amount:          trans.Amount,
		Date:            trans.Date,
		SourceName:      source,
		DestinationName: destination,
		CategoryName:    trans.Category,
		Notes:           trans.Notes,
		ExternalID:      externalID,
		BudgetName:      budget,
		Tags:            trans.Tags,
	}

	if dryRun {
		return Result{
			Row:         rowNum,
			Status:      "dry-run",
			Transaction: trans,
		}
	}

	// Check if transaction already exists
	exists, err := p.client.TransactionExists(externalID)
	if err != nil {
		return Result{
			Row:    rowNum,
			Status: "error",
			Error:  fmt.Errorf("failed to check if transaction exists: %w", err),
		}
	}

	if exists {
		return Result{
			Row:    rowNum,
			Status: "skipped (already exists)",
		}
	}

	// Create transaction
	id, err := p.client.CreateTransaction(&transData)
	if err != nil {
		// Silently skip transactions deleted by rules
		if errors.Is(err, firefly.ErrDeletedByRule) {
			return Result{
				Row:    rowNum,
				Status: "skipped (deleted by rule)",
			}
		}
		return Result{
			Row:    rowNum,
			Status: "error",
			Error:  err,
		}
	}

	return Result{
		Row:    rowNum,
		Status: "created",
		ID:     id,
	}
}

func stringOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
