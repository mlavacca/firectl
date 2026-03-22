package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/mlavacca/firectl/internal/config"
	"github.com/mlavacca/firectl/internal/firefly"
)

var rulesOutput string

// sanitizedTrigger is a portable representation of a rule trigger with
// instance-specific fields (id, created_at, updated_at) removed.
type sanitizedTrigger struct {
	Title          string `json:"title"`
	Order          int    `json:"order"`
	Active         bool   `json:"active"`
	StopProcessing bool   `json:"stop_processing"`
	Type           string `json:"type"`
	Value          string `json:"value"`
}

// sanitizedAction is a portable representation of a rule action with
// instance-specific fields (id, created_at, updated_at) removed.
type sanitizedAction struct {
	Order          int    `json:"order"`
	Active         bool   `json:"active"`
	StopProcessing bool   `json:"stop_processing"`
	Type           string `json:"type"`
	Value          string `json:"value"`
}

// sanitizedRule is a portable representation of a rule with all
// instance-specific fields (id, rule_group_id, timestamps) removed.
type sanitizedRule struct {
	RuleGroupTitle string             `json:"rule_group_title"`
	Order          int                `json:"order"`
	Title          string             `json:"title"`
	Description    string             `json:"description"`
	Trigger        string             `json:"trigger"`
	Active         bool               `json:"active"`
	Strict         bool               `json:"strict"`
	StopProcessing bool               `json:"stop_processing"`
	Triggers       []sanitizedTrigger `json:"triggers"`
	Actions        []sanitizedAction  `json:"actions"`
}

func sanitizeRules(rules []firefly.RuleRead) []sanitizedRule {
	out := make([]sanitizedRule, 0, len(rules))
	for i := range rules {
		r := &rules[i]
		triggers := make([]sanitizedTrigger, 0, len(r.Attributes.Triggers))
		for _, t := range r.Attributes.Triggers {
			triggers = append(triggers, sanitizedTrigger{
				Title:          t.Title,
				Order:          t.Order,
				Active:         t.Active,
				StopProcessing: t.StopProcessing,
				Type:           t.Type,
				Value:          t.Value,
			})
		}

		actions := make([]sanitizedAction, 0, len(r.Attributes.Actions))
		for _, a := range r.Attributes.Actions {
			actions = append(actions, sanitizedAction{
				Order:          a.Order,
				Active:         a.Active,
				StopProcessing: a.StopProcessing,
				Type:           a.Type,
				Value:          a.Value,
			})
		}

		out = append(out, sanitizedRule{
			RuleGroupTitle: r.Attributes.RuleGroupTitle,
			Order:          r.Attributes.Order,
			Title:          r.Attributes.Title,
			Description:    r.Attributes.Description,
			Trigger:        r.Attributes.Trigger,
			Active:         r.Attributes.Active,
			Strict:         r.Attributes.Strict,
			StopProcessing: r.Attributes.StopProcessing,
			Triggers:       triggers,
			Actions:        actions,
		})
	}
	return out
}

var listRulesCmd = &cobra.Command{
	Use:   "list-rules",
	Short: "List all Firefly III rules and dump them to a file or stdout",
	Long: `Fetches all rules from your Firefly III instance and writes them to
a file (via -o) or stdout if no output path is given.

Instance-specific fields (IDs, timestamps, rule_group_id) are stripped so
the output can be used as portable configuration.

Configuration:
  - FIREFLY_URL and FIREFLY_TOKEN environment variables (or .env file)

Example:
  firectl list-rules
  firectl list-rules -o rules-config.json`,

	Args: cobra.NoArgs,
	RunE: runListRules,
}

func init() {
	rootCmd.AddCommand(listRulesCmd)
	listRulesCmd.Flags().StringVarP(&rulesOutput, "output", "o", "", "Write rules to this file (default: stdout)")
}

func runListRules(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	client := firefly.NewClient(cfg.FireflyURL, cfg.FireflyToken)

	fmt.Fprintf(os.Stderr, "Fetching rules from %s...\n", cfg.FireflyURL)

	rules, err := client.ListRules()
	if err != nil {
		return fmt.Errorf("failed to list rules: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Found %d rule(s)\n", len(rules))

	data, err := json.MarshalIndent(sanitizeRules(rules), "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal rules: %w", err)
	}
	data = append(data, '\n')

	var w io.Writer = os.Stdout
	if rulesOutput != "" {
		f, err := os.OpenFile(rulesOutput, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) //nolint:gosec // path is user-supplied via -o flag
		if err != nil {
			return fmt.Errorf("failed to open %s: %w", rulesOutput, err)
		}
		defer func() { _ = f.Close() }()
		w = f
		defer fmt.Fprintf(os.Stderr, "Rules written to %s\n", rulesOutput)
	}

	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("failed to write rules: %w", err)
	}
	return nil
}
