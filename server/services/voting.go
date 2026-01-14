package services

import (
	"database/sql"
	"fmt"
	"krampus/server/config"
	"krampus/server/database"
	"krampus/server/models"
	"log"
	"time"
)

// SubmitVote submits or updates a user's vote on a proposal
func SubmitVote(userID, proposalID int64, voteType string) error {
	// Validate vote type
	if voteType != string(models.VoteTypeAllowlist) && voteType != string(models.VoteTypeBlocklist) {
		return fmt.Errorf("invalid vote type: %s", voteType)
	}

	// Check if proposal exists and is pending
	var status string
	err := database.DB.QueryRow(
		`SELECT status FROM proposals WHERE id = ?`,
		proposalID,
	).Scan(&status)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("proposal not found")
		}
		return fmt.Errorf("failed to fetch proposal: %w", err)
	}

	if status != string(models.ProposalStatusPending) {
		return fmt.Errorf("cannot vote on proposal with status: %s", status)
	}

	// Insert or update vote (upsert)
	_, err = database.DB.Exec(
		`INSERT INTO votes (user_id, proposal_id, vote_type, created_at)
		 VALUES (?, ?, ?, datetime('now'))
		 ON CONFLICT(user_id, proposal_id)
		 DO UPDATE SET vote_type = excluded.vote_type, created_at = excluded.created_at`,
		userID, proposalID, voteType,
	)
	if err != nil {
		return fmt.Errorf("failed to submit vote: %w", err)
	}

	// Recalculate vote counts
	if err := recalculateVotes(proposalID); err != nil {
		return fmt.Errorf("failed to recalculate votes: %w", err)
	}

	// Check if threshold is met
	if err := checkAndFinalizeProposal(proposalID); err != nil {
		log.Printf("Failed to finalize proposal %d: %v", proposalID, err)
		// Don't return error, as vote was recorded successfully
	}

	return nil
}

// recalculateVotes updates the vote counts for a proposal
func recalculateVotes(proposalID int64) error {
	var allowlistVotes, blocklistVotes int

	// Count allowlist votes
	err := database.DB.QueryRow(
		`SELECT COUNT(*) FROM votes WHERE proposal_id = ? AND vote_type = ?`,
		proposalID, models.VoteTypeAllowlist,
	).Scan(&allowlistVotes)
	if err != nil {
		return err
	}

	// Count blocklist votes
	err = database.DB.QueryRow(
		`SELECT COUNT(*) FROM votes WHERE proposal_id = ? AND vote_type = ?`,
		proposalID, models.VoteTypeBlocklist,
	).Scan(&blocklistVotes)
	if err != nil {
		return err
	}

	// Update proposal
	_, err = database.DB.Exec(
		`UPDATE proposals SET allowlist_votes = ?, blocklist_votes = ? WHERE id = ?`,
		allowlistVotes, blocklistVotes, proposalID,
	)
	return err
}

// checkAndFinalizeProposal checks if a proposal has reached the vote threshold
func checkAndFinalizeProposal(proposalID int64) error {
	var proposal models.Proposal
	err := database.DB.QueryRow(
		`SELECT id, identifier, rule_type, proposed_policy, custom_message,
		 created_by, status, allowlist_votes, blocklist_votes
		 FROM proposals WHERE id = ?`,
		proposalID,
	).Scan(
		&proposal.ID, &proposal.Identifier, &proposal.RuleType, &proposal.ProposedPolicy,
		&proposal.CustomMessage, &proposal.CreatedBy, &proposal.Status,
		&proposal.AllowlistVotes, &proposal.BlocklistVotes,
	)
	if err != nil {
		return err
	}

	// Skip if not pending
	if proposal.Status != string(models.ProposalStatusPending) {
		return nil
	}

	threshold := config.AppConfig.VoteThreshold

	// Check if allowlist threshold is met
	if proposal.AllowlistVotes >= threshold {
		return FinalizeProposal(proposalID, string(models.PolicyAllowlist))
	}

	// Check if blocklist threshold is met
	if proposal.BlocklistVotes >= threshold {
		return FinalizeProposal(proposalID, string(models.PolicyBlocklist))
	}

	return nil
}

// FinalizeProposal finalizes a proposal and creates a rule
func FinalizeProposal(proposalID int64, policy string) error {
	// Validate policy
	if policy != string(models.PolicyAllowlist) && policy != string(models.PolicyBlocklist) {
		return fmt.Errorf("invalid policy: %s", policy)
	}

	// Fetch proposal
	var proposal models.Proposal
	err := database.DB.QueryRow(
		`SELECT id, identifier, rule_type, custom_message, created_by, status
		 FROM proposals WHERE id = ?`,
		proposalID,
	).Scan(
		&proposal.ID, &proposal.Identifier, &proposal.RuleType,
		&proposal.CustomMessage, &proposal.CreatedBy, &proposal.Status,
	)
	if err != nil {
		return fmt.Errorf("failed to fetch proposal: %w", err)
	}

	// Check if already finalized
	if proposal.Status != string(models.ProposalStatusPending) {
		return fmt.Errorf("proposal already finalized with status: %s", proposal.Status)
	}

	// Begin transaction
	tx, err := database.DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Update proposal status
	now := time.Now()
	_, err = tx.Exec(
		`UPDATE proposals SET status = ?, finalized_at = ? WHERE id = ?`,
		models.ProposalStatusApproved, now, proposalID,
	)
	if err != nil {
		return fmt.Errorf("failed to update proposal: %w", err)
	}

	// Create rule from proposal
	// Use custom_message as the comment to identify the application
	_, err = tx.Exec(
		`INSERT INTO rules (identifier, policy, rule_type, comment, created_by, proposal_id)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		proposal.Identifier, policy, proposal.RuleType,
		proposal.CustomMessage, proposal.CreatedBy, proposalID,
	)
	if err != nil {
		return fmt.Errorf("failed to create rule: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf("Proposal %d finalized with policy %s, rule created", proposalID, policy)
	return nil
}

// AdminApproveProposal allows admin to bypass voting and directly approve a proposal
func AdminApproveProposal(proposalID int64, policy string) error {
	return FinalizeProposal(proposalID, policy)
}

// GetUserVote retrieves a user's vote for a specific proposal
func GetUserVote(userID, proposalID int64) (*models.Vote, error) {
	var vote models.Vote
	err := database.DB.QueryRow(
		`SELECT id, user_id, proposal_id, vote_type, created_at
		 FROM votes WHERE user_id = ? AND proposal_id = ?`,
		userID, proposalID,
	).Scan(&vote.ID, &vote.UserID, &vote.ProposalID, &vote.VoteType, &vote.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &vote, nil
}
