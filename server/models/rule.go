package models

import (
	"time"
)

type Rule struct {
	ID            int64      `json:"id"`
	Identifier    string     `json:"identifier"`
	Policy        string     `json:"policy"` // "ALLOWLIST" or "BLOCKLIST"
	RuleType      string     `json:"rule_type"` // "BINARY", "CERTIFICATE", "SIGNINGID", "TEAMID", "CDHASH"
	CustomMessage *string    `json:"custom_message,omitempty"`
	Comment       *string    `json:"comment,omitempty"` // Internal comment for identifying the application
	CreatedBy     *int64     `json:"created_by,omitempty"`
	ProposalID    *int64     `json:"proposal_id,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

type Policy string

const (
	PolicyAllowlist Policy = "ALLOWLIST"
	PolicyBlocklist Policy = "BLOCKLIST"
)

type RuleType string

const (
	RuleTypeBinary      RuleType = "BINARY"
	RuleTypeCertificate RuleType = "CERTIFICATE"
	RuleTypeSigningID   RuleType = "SIGNINGID"
	RuleTypeTeamID      RuleType = "TEAMID"
	RuleTypeCDHash      RuleType = "CDHASH"
)

// Santa sync protocol rule format
type SantaRule struct {
	Identifier  string  `json:"identifier"`
	Policy      string  `json:"policy"`
	RuleType    string  `json:"rule_type"`
	CustomMsg   *string `json:"custom_msg,omitempty"`
	CustomURL   *string `json:"custom_url,omitempty"`
}
