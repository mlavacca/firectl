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

// RuleTrigger represents a single trigger within a Firefly III rule
type RuleTrigger struct {
	ID             string `json:"id"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
	Title          string `json:"title"`
	Order          int    `json:"order"`
	Active         bool   `json:"active"`
	StopProcessing bool   `json:"stop_processing"`
	Type           string `json:"type"`
	Value          string `json:"value"`
}

// RuleAction represents a single action within a Firefly III rule
type RuleAction struct {
	ID             string `json:"id"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
	Order          int    `json:"order"`
	Active         bool   `json:"active"`
	StopProcessing bool   `json:"stop_processing"`
	Type           string `json:"type"`
	Value          string `json:"value"`
}

// RuleAttributes represents the attributes of a Firefly III rule
type RuleAttributes struct {
	CreatedAt      string        `json:"created_at"`
	UpdatedAt      string        `json:"updated_at"`
	RuleGroupID    string        `json:"rule_group_id"`
	RuleGroupTitle string        `json:"rule_group_title"`
	Order          int           `json:"order"`
	Title          string        `json:"title"`
	Description    string        `json:"description"`
	Trigger        string        `json:"trigger"`
	Active         bool          `json:"active"`
	Strict         bool          `json:"strict"`
	StopProcessing bool          `json:"stop_processing"`
	Triggers       []RuleTrigger `json:"triggers"`
	Actions        []RuleAction  `json:"actions"`
}

// RuleRead represents a single rule as returned by the API
type RuleRead struct {
	Type       string         `json:"type"`
	ID         string         `json:"id"`
	Attributes RuleAttributes `json:"attributes"`
}

// RuleArrayResponse represents the paginated response from listing rules
type RuleArrayResponse struct {
	Data []RuleRead `json:"data"`
	Meta struct {
		Pagination struct {
			Total       int `json:"total"`
			Count       int `json:"count"`
			PerPage     int `json:"per_page"`
			CurrentPage int `json:"current_page"`
			TotalPages  int `json:"total_pages"`
		} `json:"pagination"`
	} `json:"meta"`
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

// ListRules fetches all rules from Firefly III, paginating through all pages
func (c *Client) ListRules() ([]RuleRead, error) {
	var allRules []RuleRead
	page := 1

	for {
		endpoint := fmt.Sprintf("%s/api/v1/rules?page=%d", c.baseURL, page)

		req, err := http.NewRequest("GET", endpoint, http.NoBody)
		if err != nil {
			return nil, err
		}

		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
		req.Header.Set("Accept", "application/vnd.api+json")

		resp, err := c.client.Do(req)
		if err != nil {
			return nil, err
		}

		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("firefly III API error: %d %s\n%s",
				resp.StatusCode, resp.Status, string(body))
		}

		var result RuleArrayResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, err
		}

		allRules = append(allRules, result.Data...)

		if result.Meta.Pagination.CurrentPage >= result.Meta.Pagination.TotalPages {
			break
		}
		page++
	}

	return allRules, nil
}
