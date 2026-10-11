package handlers

import (
	"database/sql"
	"krampus/server/database"
	"krampus/server/middleware"
	"krampus/server/models"
	"krampus/server/services"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// buildProposalsFilter assembles the WHERE clause and bound args shared by
// the proposals list count and page queries from the status and search
// params. Columns are prefixed with the p alias used by both queries.
func buildProposalsFilter(c *gin.Context, pg pageParams) (string, []any) {
	var conds []string
	var args []any

	if status := c.Query("status"); status != "" {
		conds = append(conds, "p.status = ?")
		args = append(args, status)
	}
	if clause, sargs := searchClause(pg.Search,
		"p.identifier", "p.custom_message"); clause != "" {
		conds = append(conds, clause)
		args = append(args, sargs...)
	}

	if len(conds) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

// ListProposals returns a paginated, searchable page of proposals enriched
// with their creator's username and email.
func ListProposals(c *gin.Context) {
	pg := getPageParams(c)
	where, args := buildProposalsFilter(c, pg)

	var total int
	if err := database.DB.QueryRow(
		"SELECT COUNT(*) FROM proposals p"+where, args...,
	).Scan(&total); err != nil {
		log.Printf("Failed to count proposals: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch proposals"})
		return
	}

	query := `
		SELECT p.id, p.identifier, p.rule_type, p.proposed_policy, p.custom_message,
		       p.created_by, p.status, p.allowlist_votes, p.blocklist_votes,
		       p.created_at, p.finalized_at,
		       u.username, u.email
		FROM proposals p
		JOIN users u ON p.created_by = u.id` + where +
		" ORDER BY p.created_at DESC LIMIT ? OFFSET ?"
	pageArgs := append(append([]any{}, args...), pg.PerPage, pg.Offset)

	rows, err := database.DB.Query(query, pageArgs...)
	if err != nil {
		log.Printf("Failed to query proposals: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch proposals"})
		return
	}
	defer rows.Close()

	proposals := []models.ProposalWithCreator{}
	for rows.Next() {
		var p models.ProposalWithCreator
		err := rows.Scan(
			&p.ID, &p.Identifier, &p.RuleType, &p.ProposedPolicy, &p.CustomMessage,
			&p.CreatedBy, &p.Status, &p.AllowlistVotes, &p.BlocklistVotes,
			&p.CreatedAt, &p.FinalizedAt,
			&p.CreatorUsername, &p.CreatorEmail,
		)
		if err != nil {
			log.Printf("Failed to scan proposal: %v", err)
			continue
		}
		proposals = append(proposals, p)
	}

	c.JSON(http.StatusOK, pageResponse{
		Items: proposals, Total: total, Page: pg.Page, PerPage: pg.PerPage,
	})
}

// GetProposal returns a single proposal by ID
func GetProposal(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid proposal ID"})
		return
	}

	var p models.ProposalWithCreator
	err = database.DB.QueryRow(
		`SELECT p.id, p.identifier, p.rule_type, p.proposed_policy, p.custom_message,
		        p.created_by, p.status, p.allowlist_votes, p.blocklist_votes,
		        p.created_at, p.finalized_at,
		        u.username, u.email
		 FROM proposals p
		 JOIN users u ON p.created_by = u.id
		 WHERE p.id = ?`,
		id,
	).Scan(
		&p.ID, &p.Identifier, &p.RuleType, &p.ProposedPolicy, &p.CustomMessage,
		&p.CreatedBy, &p.Status, &p.AllowlistVotes, &p.BlocklistVotes,
		&p.CreatedAt, &p.FinalizedAt,
		&p.CreatorUsername, &p.CreatorEmail,
	)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Proposal not found"})
		return
	}
	if err != nil {
		log.Printf("Failed to fetch proposal: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch proposal"})
		return
	}

	// Get user's vote if authenticated
	userID, exists := middleware.GetUserID(c)
	if exists {
		vote, _ := services.GetUserVote(userID, id)
		if vote != nil {
			// Add user's vote to response (could extend model)
			c.JSON(http.StatusOK, gin.H{
				"proposal":  p,
				"user_vote": vote.VoteType,
			})
			return
		}
	}

	c.JSON(http.StatusOK, p)
}

// CreateProposal creates a new proposal
func CreateProposal(c *gin.Context) {
	userID, exists := middleware.GetUserID(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	var input struct {
		Identifier     string  `json:"identifier" binding:"required"`
		RuleType       string  `json:"rule_type" binding:"required"`
		ProposedPolicy string  `json:"proposed_policy" binding:"required"`
		CustomMessage  *string `json:"custom_message"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate rule type
	if !models.IsValidRuleType(input.RuleType) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid rule type"})
		return
	}

	// Validate policy
	if input.ProposedPolicy != string(models.PolicyAllowlist) &&
		input.ProposedPolicy != string(models.PolicyBlocklist) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid policy"})
		return
	}

	// Create proposal
	result, err := database.DB.Exec(
		`INSERT INTO proposals (identifier, rule_type, proposed_policy, custom_message, created_by)
		 VALUES (?, ?, ?, ?, ?)`,
		input.Identifier, input.RuleType, input.ProposedPolicy, input.CustomMessage, userID,
	)
	if err != nil {
		log.Printf("Failed to create proposal: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create proposal"})
		return
	}

	proposalID, _ := result.LastInsertId()

	// Fire a Discord alert for the new proposal (no-op if unconfigured; async).
	username, _ := middleware.GetUsername(c)
	var creatorEmail *string
	if email := c.GetString("email"); email != "" {
		creatorEmail = &email
	}
	services.NotifyNewProposal(models.Proposal{
		ID:             proposalID,
		Identifier:     input.Identifier,
		RuleType:       input.RuleType,
		ProposedPolicy: input.ProposedPolicy,
		CustomMessage:  input.CustomMessage,
		CreatedBy:      userID,
		CreatedAt:      time.Now(),
	}, username, creatorEmail)

	c.JSON(http.StatusCreated, gin.H{
		"id":      proposalID,
		"message": "Proposal created successfully",
	})
}

// VoteOnProposal submits a vote on a proposal
func VoteOnProposal(c *gin.Context) {
	userID, exists := middleware.GetUserID(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	proposalID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid proposal ID"})
		return
	}

	var input struct {
		VoteType string `json:"vote_type" binding:"required"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Submit vote
	err = services.SubmitVote(userID, proposalID, input.VoteType)
	if err != nil {
		log.Printf("Failed to submit vote: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Vote submitted successfully"})
}

// ApproveProposal allows admin to directly approve a proposal
func ApproveProposal(c *gin.Context) {
	proposalID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid proposal ID"})
		return
	}

	var input struct {
		Policy string `json:"policy" binding:"required"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Admin approve
	err = services.AdminApproveProposal(proposalID, input.Policy)
	if err != nil {
		log.Printf("Failed to approve proposal: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Proposal approved successfully"})
}

// RejectProposal allows admin to reject a pending proposal without creating a rule
func RejectProposal(c *gin.Context) {
	proposalID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid proposal ID"})
		return
	}

	if err := services.RejectProposal(proposalID); err != nil {
		log.Printf("Failed to reject proposal: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Proposal rejected"})
}

// voteRecord is one row of a proposal's vote history.
type voteRecord struct {
	Username  string    `json:"username"`
	VoteType  string    `json:"vote_type"`
	CreatedAt time.Time `json:"created_at"`
}

// ListProposalVotes returns the per-user vote history for a proposal.
func ListProposalVotes(c *gin.Context) {
	proposalID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid proposal ID"})
		return
	}

	var exists int
	err = database.DB.QueryRow(`SELECT 1 FROM proposals WHERE id = ?`, proposalID).Scan(&exists)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Proposal not found"})
		return
	}
	if err != nil {
		log.Printf("Failed to fetch proposal: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch proposal"})
		return
	}

	rows, err := database.DB.Query(
		`SELECT v.vote_type, v.created_at, u.username
		 FROM votes v JOIN users u ON u.id = v.user_id
		 WHERE v.proposal_id = ? ORDER BY v.created_at`,
		proposalID,
	)
	if err != nil {
		log.Printf("Failed to query votes: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch votes"})
		return
	}
	defer rows.Close()

	votes := []voteRecord{}
	for rows.Next() {
		var v voteRecord
		if err := rows.Scan(&v.VoteType, &v.CreatedAt, &v.Username); err != nil {
			log.Printf("Failed to scan vote: %v", err)
			continue
		}
		votes = append(votes, v)
	}

	c.JSON(http.StatusOK, gin.H{"votes": votes})
}

// DeleteProposal deletes a proposal (creator or admin only)
func DeleteProposal(c *gin.Context) {
	userID, exists := middleware.GetUserID(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	role, _ := middleware.GetRole(c)
	proposalID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid proposal ID"})
		return
	}

	// Check if user is creator or admin
	var createdBy int64
	err = database.DB.QueryRow(`SELECT created_by FROM proposals WHERE id = ?`, proposalID).Scan(&createdBy)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Proposal not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch proposal"})
		return
	}

	if createdBy != userID && role != string(models.RoleAdmin) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Only the creator or admin can delete this proposal"})
		return
	}

	// Delete proposal (votes will be cascade deleted)
	_, err = database.DB.Exec(`DELETE FROM proposals WHERE id = ?`, proposalID)
	if err != nil {
		log.Printf("Failed to delete proposal: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete proposal"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Proposal deleted successfully"})
}
