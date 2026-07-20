package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"krampus/server/config"
	"krampus/server/models"
	"log"
	"net/http"
	"strings"
	"time"
)

// discordEmbedField is a single name/value row in a Discord embed.
type discordEmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

type discordEmbed struct {
	Title       string              `json:"title"`
	Description string              `json:"description,omitempty"`
	URL         string              `json:"url,omitempty"`
	Color       int                 `json:"color"`
	Fields      []discordEmbedField `json:"fields,omitempty"`
	Timestamp   string              `json:"timestamp,omitempty"`
}

type discordWebhookPayload struct {
	Username string         `json:"username,omitempty"`
	Embeds   []discordEmbed `json:"embeds"`
}

// discordHTTPClient is package-level so tests can observe/replace it if needed.
var discordHTTPClient = &http.Client{Timeout: 10 * time.Second}

// NotifyNewProposal sends a Discord alert for a newly created proposal. It is a
// no-op when no webhook is configured and runs in the background so it never
// blocks or fails the request that created the proposal.
func NotifyNewProposal(p models.Proposal, creatorName string, creatorEmail *string) {
	webhook := config.AppConfig.DiscordWebhookURL
	if webhook == "" {
		return
	}

	payload := buildNewProposalPayload(p, creatorName, creatorEmail, config.AppConfig.SyncBaseURL)

	go func() {
		if err := postDiscordWebhook(webhook, payload); err != nil {
			log.Printf("Discord notification failed for proposal %d: %v", p.ID, err)
		}
	}()
}

// buildNewProposalPayload constructs the webhook body. Split out so it is unit
// testable without making any network calls.
func buildNewProposalPayload(p models.Proposal, creatorName string, creatorEmail *string, baseURL string) discordWebhookPayload {
	// Green for allowlist proposals, red for blocklist.
	color := 0x2ecc71
	if p.ProposedPolicy == string(models.PolicyBlocklist) {
		color = 0xe74c3c
	}

	requestedBy := creatorName
	if creatorEmail != nil && *creatorEmail != "" {
		requestedBy = fmt.Sprintf("%s (%s)", creatorName, *creatorEmail)
	}

	fields := []discordEmbedField{
		{Name: "Identifier", Value: fmt.Sprintf("`%s`", p.Identifier), Inline: false},
		{Name: "Rule Type", Value: p.RuleType, Inline: true},
		{Name: "Proposed Policy", Value: p.ProposedPolicy, Inline: true},
		{Name: "Requested By", Value: requestedBy, Inline: false},
	}
	if p.CustomMessage != nil && *p.CustomMessage != "" {
		fields = append(fields, discordEmbedField{Name: "Message", Value: *p.CustomMessage, Inline: false})
	}

	embed := discordEmbed{
		Title:     "New Santa rule proposal",
		Color:     color,
		Fields:    fields,
		Timestamp: p.CreatedAt.UTC().Format(time.RFC3339),
	}
	if baseURL != "" {
		embed.URL = strings.TrimRight(baseURL, "/") + "/proposals"
	}

	return discordWebhookPayload{
		Username: "Krampus",
		Embeds:   []discordEmbed{embed},
	}
}

func postDiscordWebhook(webhookURL string, payload discordWebhookPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := discordHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("discord returned status %d", resp.StatusCode)
	}
	return nil
}
