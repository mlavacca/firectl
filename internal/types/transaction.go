package types

// Transaction represents a parsed transaction from CSV
type Transaction struct {
	Date        string
	Description string
	Amount      string
	Type        *string
	Source      *string
	Destination *string
	Category    *string
	Notes       *string
	Budget      *string
	Tags        []string
	Status      *string
}
