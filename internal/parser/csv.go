package parser

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mlavacca/firectl/internal/parser/providers"
	"github.com/mlavacca/firectl/internal/types"
)

func init() {
	// Register all providers
	RegisterProvider(providers.NewSatispayProvider())
	RegisterProvider(providers.NewSanpaoloProvider())
}

// ParseCSV parses a CSV file and returns a slice of transactions
func ParseCSV(filePath, providerName string) ([]types.Transaction, error) {
	// Get the specified provider
	provider := GetProvider(providerName)
	if provider == nil {
		availableProviders := ListProviders()
		return nil, fmt.Errorf("unknown provider: %s. Available providers: %v", providerName, availableProviders)
	}

	// #nosec G304 -- file path is provided by the user as a CLI argument
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer func() { _ = file.Close() }()

	// Read all lines to detect header
	allLines, err := readAllLines(file)
	if err != nil {
		return nil, err
	}

	// Find header row
	headerIdx, columns, err := findHeaderRow(allLines, provider)
	if err != nil {
		return nil, err
	}

	// Parse CSV starting from header row
	_, _ = file.Seek(0, 0) // Reset file pointer
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1 // Allow variable number of fields
	reader.TrimLeadingSpace = true

	// Skip lines before header
	for i := 0; i < headerIdx; i++ {
		_, err = reader.Read()
		if err != nil {
			return nil, fmt.Errorf("failed to skip line %d: %w", i, err)
		}
	}

	// Read header
	_, err = reader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	// Use the specified provider
	fmt.Fprintf(os.Stderr, "Using provider: %s\n", provider.Name())
	return parseWithProvider(reader, provider, columns, headerIdx)
}

func readAllLines(file *os.File) ([][]string, error) {
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true

	var lines [][]string
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		lines = append(lines, record)
	}
	return lines, nil
}

func findHeaderRow(lines [][]string, provider Provider) (idx int, headers []string, err error) {
	for i, line := range lines {
		if isEmptyRow(line) {
			continue
		}

		// Check if the provider can handle this row
		if provider.CanHandle(line) {
			return i, line, nil
		}
	}

	return -1, nil, fmt.Errorf("could not find header row compatible with provider: %s", provider.Name())
}

func isEmptyRow(record []string) bool {
	for _, field := range record {
		if strings.TrimSpace(field) != "" {
			return false
		}
	}
	return true
}
