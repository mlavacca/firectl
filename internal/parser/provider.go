package parser

import (
	"encoding/csv"
	"fmt"
	"strings"

	"github.com/mlavacca/firectl/internal/types"
)

// Provider defines the interface for bank-specific CSV parsers
type Provider interface {
	// Name returns the provider name (e.g., "satispay", "intesa-san-paolo")
	Name() string

	// CanHandle checks if this provider can handle the given CSV headers
	CanHandle(headers []string) bool

	// Parse parses a CSV record into a Transaction
	// columnMap maps column names to their indices
	Parse(record []string, columnMap map[string]int, rowNum int) (types.Transaction, error)
}

// ProviderRegistry holds all registered providers
type ProviderRegistry struct {
	providers []Provider
}

// NewProviderRegistry creates a new provider registry
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make([]Provider, 0),
	}
}

// Register adds a provider to the registry
func (r *ProviderRegistry) Register(provider Provider) {
	r.providers = append(r.providers, provider)
}

// GetProvider returns a provider by name
func (r *ProviderRegistry) GetProvider(name string) Provider {
	for _, provider := range r.providers {
		if provider.Name() == name {
			return provider
		}
	}
	return nil
}

// Global provider registry
var defaultRegistry = NewProviderRegistry()

// RegisterProvider registers a provider in the default registry
func RegisterProvider(provider Provider) {
	defaultRegistry.Register(provider)
}

// GetProvider returns a provider by name
func GetProvider(name string) Provider {
	return defaultRegistry.GetProvider(name)
}

// ListProviders returns all registered provider names
func ListProviders() []string {
	names := make([]string, 0, len(defaultRegistry.providers))
	for _, p := range defaultRegistry.providers {
		names = append(names, p.Name())
	}
	return names
}

// buildColumnMapFromHeaders creates a column index map from header names
func buildColumnMapFromHeaders(headers []string) map[string]int {
	colMap := make(map[string]int)
	for i, header := range headers {
		// Trim whitespace from header names to handle inconsistent CSV formatting
		trimmedHeader := strings.TrimSpace(header)
		colMap[trimmedHeader] = i
	}
	return colMap
}

// parseWithProvider parses CSV using a specific provider
func parseWithProvider(reader *csv.Reader, provider Provider, headers []string, headerIdx int) ([]types.Transaction, error) {
	var transactions []types.Transaction

	// Build column map
	colMap := buildColumnMapFromHeaders(headers)

	// Parse data rows
	rowNum := headerIdx + 2 // +1 for header, +1 for next row

	for {
		record, err := reader.Read()
		if err != nil {
			break // EOF or error
		}

		// Skip empty rows
		if isEmptyRow(record) {
			rowNum++
			continue
		}

		transaction, err := provider.Parse(record, colMap, rowNum)
		if err != nil {
			fmt.Printf("Warning: Row %d: %v\n", rowNum, err)
			rowNum++
			continue
		}

		// Skip if transaction should be skipped (e.g., non-accounted)
		if transaction.Status != nil && *transaction.Status == "NON CONTABILIZZATO" {
			rowNum++
			continue
		}

		transactions = append(transactions, transaction)
		rowNum++
	}

	return transactions, nil
}
