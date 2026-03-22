package providers

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/mlavacca/firectl/internal/types"
)

// SanpaoloProvider handles Intesa Sanpaolo CSV format
type SanpaoloProvider struct{}

// NewSanpaoloProvider creates a new Sanpaolo provider
func NewSanpaoloProvider() *SanpaoloProvider {
	return &SanpaoloProvider{}
}

// Name returns the provider name
func (p *SanpaoloProvider) Name() string {
	return "sanpaolo"
}

// CanHandle checks if this provider can handle the given CSV headers
func (p *SanpaoloProvider) CanHandle(headers []string) bool {
	// Sanpaolo has specific column names
	requiredColumns := []string{"Data", "Operazione", "Importo", "Contabilizzazione"}

	foundCount := 0
	for _, required := range requiredColumns {
		for _, header := range headers {
			if strings.TrimSpace(header) == required {
				foundCount++
				break
			}
		}
	}

	// If we find all required columns, this is a Sanpaolo CSV
	return foundCount == len(requiredColumns)
}

// Parse parses a Sanpaolo CSV record into a Transaction
func (p *SanpaoloProvider) Parse(record []string, columnMap map[string]int, rowNum int) (types.Transaction, error) {
	trans := types.Transaction{}

	// Get column indices
	dataIdx, hasData := columnMap["Data"]
	operazioneIdx, hasOperazione := columnMap["Operazione"]
	dettagliIdx, hasDettagli := columnMap["Dettagli"]
	contoIdx, hasConto := columnMap["Conto o carta"]
	contabilizzazioneIdx, hasContabilizzazione := columnMap["Contabilizzazione"]
	categoriaIdx, hasCategoria := columnMap["Categoria"]
	importoIdx, hasImporto := columnMap["Importo"]

	if !hasData || !hasOperazione || !hasImporto {
		return trans, fmt.Errorf("missing required columns")
	}

	if dataIdx >= len(record) || operazioneIdx >= len(record) || importoIdx >= len(record) {
		return trans, fmt.Errorf("record has fewer columns than expected")
	}

	// Skip non-contabilizzato transactions
	if hasContabilizzazione && contabilizzazioneIdx < len(record) {
		stato := strings.TrimSpace(record[contabilizzazioneIdx])
		if stato != "CONTABILIZZATO" {
			return trans, fmt.Errorf("skipping non-accounted transaction")
		}
	}

	// Parse date: "01/31/2026" -> MM/DD/YYYY
	dateStr := strings.TrimSpace(record[dataIdx])
	parsedDate, err := p.parseDate(dateStr)
	if err != nil {
		return trans, fmt.Errorf("failed to parse date: %w", err)
	}
	trans.Date = parsedDate

	// Get operation/merchant name
	operazione := strings.TrimSpace(record[operazioneIdx])
	description := operazione

	// If there are details, append them
	if hasDettagli && dettagliIdx < len(record) {
		dettagli := strings.TrimSpace(record[dettagliIdx])
		if dettagli != "" && dettagli != operazione {
			// Dettagli is often very long, so we just use operazione for description
			// but we can add dettagli as notes
			notes := dettagli
			trans.Notes = &notes
		}
	}
	trans.Description = description

	// Parse amount
	amountStr := strings.TrimSpace(record[importoIdx])
	amount, transType, err := p.parseAmount(amountStr)
	if err != nil {
		return trans, fmt.Errorf("failed to parse amount: %w", err)
	}
	trans.Amount = amount
	trans.Type = &transType

	// Extract account from "Conto o carta" field
	accountName := "Sanpaolo"
	if hasConto && contoIdx < len(record) {
		contoStr := strings.TrimSpace(record[contoIdx])
		// Extract account number: "Conto 1000/00079347" -> "1000/00079347"
		if strings.Contains(contoStr, "Conto ") {
			parts := strings.Split(contoStr, "Conto ")
			if len(parts) > 1 {
				accountName = "Sanpaolo " + strings.TrimSpace(parts[1])
			}
		}
	}

	// Set source/destination based on transaction type
	if transType == transactionTypeDeposit {
		// Money coming in
		source := p.determineSource(operazione)
		destination := accountName
		trans.Source = &source
		trans.Destination = &destination
	} else {
		// Money going out (withdrawal)
		source := accountName
		destination := p.determineDestination(operazione)
		trans.Source = &source
		trans.Destination = &destination
	}

	// Parse category
	if hasCategoria && categoriaIdx < len(record) {
		categoria := strings.TrimSpace(record[categoriaIdx])
		if categoria != "" {
			trans.Category = &categoria
		}
	}

	return trans, nil
}

// parseDate parses Sanpaolo date format: MM/DD/YYYY
func (p *SanpaoloProvider) parseDate(dateStr string) (string, error) {
	// Parse: "01/31/2026"
	t, err := time.Parse("01/02/2006", dateStr)
	if err != nil {
		return "", err
	}

	// Convert to Firefly format: YYYY-MM-DDTHH:MM:SS+00:00 (ISO 8601)
	year := t.Year()
	day := t.Day()
	month := int(t.Month())

	return fmt.Sprintf("%04d-%02d-%02dT00:00:00+00:00",
		year, month, day), nil
}

// parseAmount parses the amount and determines transaction type
func (p *SanpaoloProvider) parseAmount(amountStr string) (result, txType string, parseErr error) {
	// Remove any non-numeric characters except . , and -
	cleaned := strings.Map(func(r rune) rune {
		if (r >= '0' && r <= '9') || r == '.' || r == '-' || r == ',' {
			return r
		}
		return -1
	}, amountStr)

	// Replace comma with dot for decimal separator
	cleaned = strings.ReplaceAll(cleaned, ",", ".")

	amount, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		return "", "", err
	}

	// Determine type based on sign
	transType := "withdrawal"
	if amount > 0 {
		transType = "deposit"
	}

	// Return absolute value
	absAmount := math.Abs(amount)
	return fmt.Sprintf("%.2f", absAmount), transType, nil
}

// determineSource determines the source account for deposits
func (p *SanpaoloProvider) determineSource(operazione string) string {
	lower := strings.ToLower(operazione)

	if strings.Contains(lower, "bonifico disposto da") {
		// Extract sender from operation name
		parts := strings.Split(operazione, "Bonifico Disposto Da ")
		if len(parts) > 1 {
			return strings.TrimSpace(parts[1])
		}
	}

	if strings.Contains(lower, "disposizione di giroconto") {
		return "Internal Transfer"
	}

	if strings.Contains(lower, "stipendio") || strings.Contains(lower, "stipendi") {
		return "Employer"
	}

	// Default: use the operation name as source
	return operazione
}

// determineDestination determines the destination for withdrawals
func (p *SanpaoloProvider) determineDestination(operazione string) string {
	lower := strings.ToLower(operazione)

	if strings.Contains(lower, "bonifico") && (strings.Contains(lower, "a favore di") || strings.Contains(lower, "disposto a favore di")) {
		// Extract recipient from operation name
		if strings.Contains(operazione, "A Favore Di ") {
			parts := strings.Split(operazione, "A Favore Di ")
			if len(parts) > 1 {
				return strings.TrimSpace(parts[1])
			}
		}
		if strings.Contains(operazione, "a favore di ") {
			parts := strings.Split(operazione, "a favore di ")
			if len(parts) > 1 {
				return strings.TrimSpace(parts[1])
			}
		}
	}

	if strings.Contains(lower, "addebito diretto disposto a favore di") {
		parts := strings.Split(operazione, "A Favore Di ")
		if len(parts) > 1 {
			recipient := strings.Split(parts[1], " MANDATO")[0]
			return strings.TrimSpace(recipient)
		}
	}

	if strings.Contains(lower, "disposizione di giroconto") {
		return "Giroconto"
	}

	if strings.Contains(lower, "prelievo sportello banca del gruppo") {
		return "Prelievo"
	}

	// Default: use the operation name as destination
	return operazione
}
