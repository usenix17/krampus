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
	"time"

	"github.com/gin-gonic/gin"
)

// ListProposals returns all proposals with optional filtering
func ListProposals(c *gin.Context) {
	status := c.Query("status") // Filter by status if provided

	query := `
		SELECT p.id, p.identifier, p.rule_type, p.proposed_policy, p.custom_message,
		       p.created_by, p.status, p.allowlist_votes, p.blocklist_votes,
		       p.created_at, p.finalized_at,
		       u.username, u.email
		FROM proposals p
		JOIN users u ON p.created_by = u.id
	`

	var rows *sql.Rows
	var err error

	if status != "" {
		query += " WHERE p.status = ? ORDER BY p.created_at DESC"
		rows, err = database.DB.Query(query, status)
	} else {
		query += " ORDER BY p.created_at DESC"
		rows, err = database.DB.Query(query)
	}

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

	c.JSON(http.StatusOK, proposals)
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
