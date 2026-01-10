package handlers

import (
	"database/sql"
	"krampus/server/database"
	"krampus/server/middleware"
	"krampus/server/models"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ListRules returns all active rules
func ListRules(c *gin.Context) {
	policy := c.Query("policy")     // Filter by policy
	ruleType := c.Query("rule_type") // Filter by rule type

	query := `
		SELECT r.id, r.identifier, r.policy, r.rule_type, r.custom_message,
		       r.comment, r.created_by, r.proposal_id, r.created_at
		FROM rules r
		WHERE 1=1
	`
	args := []interface{}{}

	if policy != "" {
		query += " AND r.policy = ?"
		args = append(args, policy)
	}
	if ruleType != "" {
		query += " AND r.rule_type = ?"
		args = append(args, ruleType)
	}

	query += " ORDER BY r.created_at DESC"

	rows, err := database.DB.Query(query, args...)
	if err != nil {
		log.Printf("Failed to query rules: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch rules"})
		return
	}
	defer rows.Close()

	rules := []models.Rule{}
	for rows.Next() {
		var r models.Rule
		err := rows.Scan(
			&r.ID, &r.Identifier, &r.Policy, &r.RuleType, &r.CustomMessage,
			&r.Comment, &r.CreatedBy, &r.ProposalID, &r.CreatedAt,
		)
		if err != nil {
			log.Printf("Failed to scan rule: %v", err)
			continue
		}
		rules = append(rules, r)
	}

	c.JSON(http.StatusOK, rules)
}

// GetRule returns a single rule by ID
func GetRule(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid rule ID"})
		return
	}

	var r models.Rule
	err = database.DB.QueryRow(
		`SELECT id, identifier, policy, rule_type, custom_message, comment, created_by, proposal_id, created_at
		 FROM rules WHERE id = ?`,
		id,
	).Scan(&r.ID, &r.Identifier, &r.Policy, &r.RuleType, &r.CustomMessage, &r.Comment, &r.CreatedBy, &r.ProposalID, &r.CreatedAt)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Rule not found"})
		return
	}
	if err != nil {
		log.Printf("Failed to fetch rule: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch rule"})
		return
	}

	c.JSON(http.StatusOK, r)
}

// CreateRule creates a new rule directly (admin only)
func CreateRule(c *gin.Context) {
	userID, exists := middleware.GetUserID(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	var input struct {
		Identifier    string  `json:"identifier" binding:"required"`
		RuleType      string  `json:"rule_type" binding:"required"`
		Policy        string  `json:"policy" binding:"required"`
		CustomMessage *string `json:"custom_message"`
		Comment       *string `json:"comment"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate policy
	if input.Policy != string(models.PolicyAllowlist) && input.Policy != string(models.PolicyBlocklist) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid policy"})
		return
	}

	// Create rule
	result, err := database.DB.Exec(
		`INSERT INTO rules (identifier, policy, rule_type, custom_message, comment, created_by)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		input.Identifier, input.Policy, input.RuleType, input.CustomMessage, input.Comment, userID,
	)
	if err != nil {
		log.Printf("Failed to create rule: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create rule"})
		return
	}

	ruleID, _ := result.LastInsertId()

	c.JSON(http.StatusCreated, gin.H{
		"id":      ruleID,
		"message": "Rule created successfully",
	})
}

// DeleteRule deletes a rule (admin only)
func DeleteRule(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid rule ID"})
		return
	}

	result, err := database.DB.Exec(`DELETE FROM rules WHERE id = ?`, id)
	if err != nil {
		log.Printf("Failed to delete rule: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete rule"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Rule not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Rule deleted successfully"})
}
