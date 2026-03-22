package firefly

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// ErrDeletedByRule is returned when a transaction was created but deleted by a Firefly rule
var ErrDeletedByRule = errors.New("transaction was deleted by a Firefly rule")

// Client represents a Firefly III API client
type Client struct {
	baseURL string
	token   string
	client  *http.Client
}

// NewClient creates a new Firefly III API client
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		client:  &http.Client{},
	}
}

// TransactionData represents a single transaction for Firefly III
type TransactionData struct {
	Type            string   `json:"type"`
	Description     string   `json:"description"`
	Amount          string   `json:"amount"`
	Date            string   `json:"date"`
	SourceName      *string  `json:"source_name,omitempty"`
	DestinationName *string  `json:"destination_name,omitempty"`
	CategoryName    *string  `json:"category_name,omitempty"`
	Notes           *string  `json:"notes,omitempty"`
	ExternalID      string   `json:"external_id"`
	BudgetName      *string  `json:"budget_name,omitempty"`
	Tags            []string `json:"tags,omitempty"`
}

// TransactionRequest represents a request to create transactions
type TransactionRequest struct {
	ErrorIfDuplicateHash bool              `json:"error_if_duplicate_hash"`
	ApplyRules           bool              `json:"apply_rules"`
	Transactions         []TransactionData `json:"transactions"`
}

// TransactionResponse represents the response from creating a transaction
type TransactionResponse struct {
	Data struct {
		ID string `json:"id"`
	} `json:"data"`
}

// SearchResponse represents the response from searching transactions
type SearchResponse struct {
	Data []interface{} `json:"data"`
}

// TransactionExists checks if a transaction with the given external_id exists
func (c *Client) TransactionExists(externalID string) (bool, error) {
	endpoint := fmt.Sprintf("%s/api/v1/search/transactions?query=%s",
		c.baseURL,
		url.QueryEscape(fmt.Sprintf("external_id:%s", externalID)))

	req, err := http.NewRequest("GET", endpoint, http.NoBody)
	if err != nil {
		return false, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return false, nil
	}

	var searchResp SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return false, err
	}

	return len(searchResp.Data) > 0, nil
}

// CreateTransaction creates a new transaction in Firefly III
func (c *Client) CreateTransaction(data *TransactionData) (string, error) {
	req := TransactionRequest{
		ErrorIfDuplicateHash: false,
		ApplyRules:           true,
		Transactions:         []TransactionData{*data},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}

	endpoint := fmt.Sprintf("%s/api/v1/transactions", c.baseURL)
	httpReq, err := http.NewRequest("POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		bodyStr := string(bodyBytes)

		// Return sentinel error for transactions deleted by rules
		if strings.Contains(bodyStr, "Possibly, a rule deleted this transaction after its creation") {
			return "", ErrDeletedByRule
		}

		return "", fmt.Errorf("firefly III API error: %d %s\n%s",
			resp.StatusCode, resp.Status, bodyStr)
	}

	var transResp TransactionResponse
	if err := json.NewDecoder(resp.Body).Decode(&transResp); err != nil {
		return "", err
	}

	return transResp.Data.ID, nil
}

// GenerateExternalID generates a SHA256 hash for deduplication
func GenerateExternalID(date, amount, description, source, destination, notes string) string {
	hashInput := fmt.Sprintf("%s|%s|%s|%s|%s|%s",
		date, amount, description, source, destination, notes)
	hash := sha256.Sum256([]byte(hashInput))
	return fmt.Sprintf("%x", hash)
}
