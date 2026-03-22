package providers

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/mlavacca/firectl/internal/types"
)

const (
	transactionTypeWithdrawal = "withdrawal"
	transactionTypeDeposit    = "deposit"
	satispayAccountName       = "Satispay"
)

// SatispayProvider handles Satispay CSV format
type SatispayProvider struct{}

// NewSatispayProvider creates a new Satispay provider
func NewSatispayProvider() *SatispayProvider {
	return &SatispayProvider{}
}

// Name returns the provider name
func (p *SatispayProvider) Name() string {
	return "satispay"
}

// CanHandle checks if this provider can handle the given CSV headers
func (p *SatispayProvider) CanHandle(headers []string) bool {
	// Satispay has specific column names
	requiredColumns := []string{"Data", "Nome", "Importo", "Tipo", "Stato"}

	foundCount := 0
	for _, required := range requiredColumns {
		for _, header := range headers {
			if strings.TrimSpace(header) == required {
				foundCount++
				break
			}
		}
	}

	// If we find all required columns, this is a Satispay CSV
	return foundCount == len(requiredColumns)
}

// Parse parses a Satispay CSV record into a Transaction
func (p *SatispayProvider) Parse(record []string, columnMap map[string]int, rowNum int) (types.Transaction, error) {
	trans := types.Transaction{}

	// Get column indices
	dataIdx, hasData := columnMap["Data"]
	nomeIdx, hasNome := columnMap["Nome"]
	descrizioneIdx, hasDescrizione := columnMap["Descrizione"]
	importoIdx, hasImporto := columnMap["Importo"]
	tipoIdx, hasTipo := columnMap["Tipo"]
	statoIdx, hasStato := columnMap["Stato"]

	if !hasData || !hasNome || !hasImporto {
		return trans, fmt.Errorf("missing required columns")
	}

	if dataIdx >= len(record) || nomeIdx >= len(record) || importoIdx >= len(record) {
		return trans, fmt.Errorf("record has fewer columns than expected")
	}

	// Parse date: "01/22/2026 13:23:01" -> MM/DD/YYYY HH:MM:SS
	dateStr := strings.TrimSpace(record[dataIdx])
	parsedDate, err := p.parseDate(dateStr)
	if err != nil {
		return trans, fmt.Errorf("failed to parse date: %w", err)
	}
	trans.Date = parsedDate

	// Build description from Nome and Descrizione
	nome := strings.TrimSpace(record[nomeIdx])
	description := nome

	if hasDescrizione && descrizioneIdx < len(record) {
		desc := strings.TrimSpace(record[descrizioneIdx])
		if desc != "" {
			description = fmt.Sprintf("%s - %s", nome, desc)
		}
	}
	trans.Description = description

	// Parse amount
	amountStr := strings.TrimSpace(record[importoIdx])
	amount, err := p.parseAmount(amountStr)
	if err != nil {
		return trans, fmt.Errorf("failed to parse amount: %w", err)
	}
	trans.Amount = amount

	// Parse type (optional)
	if hasTipo && tipoIdx < len(record) {
		tipo := strings.TrimSpace(record[tipoIdx])
		if tipo != "" {
			// Extract type from emoji string: "🏬 a un Negozio" -> "withdrawal"
			transType := p.mapTipoToType(tipo)
			trans.Type = &transType
		}
	}

	// Parse stato (status)
	if hasStato && statoIdx < len(record) {
		stato := strings.TrimSpace(record[statoIdx])
		if stato != "" {
			// Remove emoji: "✅ Approvato" -> "Approvato"
			stato = strings.TrimSpace(strings.ReplaceAll(stato, "✅", ""))
			stato = strings.TrimSpace(strings.ReplaceAll(stato, "❌", ""))
			trans.Status = &stato
		}
	}

	// Set source/destination based on transaction type
	// Determine transaction type
	transType := transactionTypeWithdrawal
	if trans.Type != nil {
		transType = *trans.Type
	}

	if transType == transactionTypeDeposit {
		// For deposits (money into Satispay from bank)
		// Source: Bank account, Destination: Satispay
		source := "Bank Account"
		destination := satispayAccountName
		trans.Source = &source
		trans.Destination = &destination
	} else {
		// For withdrawals (payments from Satispay to merchants/people)
		// Source: Satispay, Destination: merchant/person name
		source := satispayAccountName
		destination := nome // Use the Nome field as destination
		trans.Source = &source
		trans.Destination = &destination
	}

	// Add category based on type
	if hasTipo && tipoIdx < len(record) {
		tipo := strings.TrimSpace(record[tipoIdx])
		category := p.extractCategory(tipo)
		if category != "" {
			trans.Category = &category
		}
	}

	return trans, nil
}

// parseDate parses Satispay date format: MM/DD/YYYY HH:MM:SS
func (p *SatispayProvider) parseDate(dateStr string) (string, error) {
	// Parse: "01/22/2026 13:23:01"
	t, err := time.Parse("01/02/2006 15:04:05", dateStr)
	if err != nil {
		return "", err
	}

	// Convert to Firefly format: YYYY-MM-DDTHH:MM:SS+00:00 (ISO 8601)
	year := t.Year()
	day := t.Day()
	month := int(t.Month())
	hour := t.Hour()
	minute := t.Minute()
	second := t.Second()

	return fmt.Sprintf("%04d-%02d-%02dT%02d:%02d:%02d+00:00",
		year, month, day, hour, minute, second), nil
}

// parseAmount parses the amount and returns absolute value as string
func (p *SatispayProvider) parseAmount(amountStr string) (string, error) {
	// Remove any non-numeric characters except . and -
	cleaned := strings.Map(func(r rune) rune {
		if (r >= '0' && r <= '9') || r == '.' || r == '-' {
			return r
		}
		return -1
	}, amountStr)

	amount, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		return "", err
	}

	// Return absolute value
	absAmount := math.Abs(amount)
	return fmt.Sprintf("%.2f", absAmount), nil
}

// mapTipoToType maps Satispay "Tipo" field to Firefly transaction type
func (p *SatispayProvider) mapTipoToType(tipo string) string {
	tipo = strings.ToLower(tipo)

	if strings.Contains(tipo, "negozio") || strings.Contains(tipo, "🏬") {
		return transactionTypeWithdrawal
	}
	if strings.Contains(tipo, "persona") || strings.Contains(tipo, "👤") {
		return transactionTypeWithdrawal
	}
	if strings.Contains(tipo, "banca") || strings.Contains(tipo, "🏦") || strings.Contains(tipo, "ricarica") {
		return transactionTypeDeposit
	}

	return transactionTypeWithdrawal // Default
}

// extractCategory extracts a clean category from the Tipo field
func (p *SatispayProvider) extractCategory(tipo string) string {
	// Remove emojis and clean up
	tipo = strings.ReplaceAll(tipo, "🏬", "")
	tipo = strings.ReplaceAll(tipo, "👤", "")
	tipo = strings.ReplaceAll(tipo, "🏦", "")
	tipo = strings.TrimSpace(tipo)

	if strings.Contains(strings.ToLower(tipo), "negozio") {
		return "Negozio"
	}
	if strings.Contains(strings.ToLower(tipo), "persona") {
		return "Trasferimento a persona"
	}
	if strings.Contains(strings.ToLower(tipo), "banca") || strings.Contains(strings.ToLower(tipo), "ricarica") {
		return "Ricarica"
	}

	return tipo
}
