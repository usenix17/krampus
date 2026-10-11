package handlers

import (
	"database/sql"
	"krampus/server/database"
	"krampus/server/middleware"
	"krampus/server/models"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// buildRulesFilter assembles the WHERE clause and bound args shared by the
// rules list count and page queries from the policy, rule_type, and search
// params. The returned clause includes the leading " WHERE " or is empty.
func buildRulesFilter(c *gin.Context, p pageParams) (string, []any) {
	var conds []string
	var args []any

	if policy := c.Query("policy"); policy != "" {
		conds = append(conds, "policy = ?")
		args = append(args, policy)
	}
	if ruleType := c.Query("rule_type"); ruleType != "" {
		conds = append(conds, "rule_type = ?")
		args = append(args, ruleType)
	}
	if clause, sargs := searchClause(p.Search,
		"identifier", "comment", "custom_message"); clause != "" {
		conds = append(conds, clause)
		args = append(args, sargs...)
	}

	if len(conds) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

// ListRules returns a paginated, searchable page of rules.
func ListRules(c *gin.Context) {
	p := getPageParams(c)
	where, args := buildRulesFilter(c, p)

	var total int
	if err := database.DB.QueryRow(
		"SELECT COUNT(*) FROM rules"+where, args...,
	).Scan(&total); err != nil {
		log.Printf("Failed to count rules: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch rules"})
		return
	}

	query := `SELECT id, identifier, policy, rule_type, custom_message,
	                 comment, created_by, proposal_id, created_at
	          FROM rules` + where + " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	pageArgs := append(append([]any{}, args...), p.PerPage, p.Offset)

	rows, err := database.DB.Query(query, pageArgs...)
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

	c.JSON(http.StatusOK, pageResponse{
		Items: rules, Total: total, Page: p.Page, PerPage: p.PerPage,
	})
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

	// Validate rule type
	if !models.IsValidRuleType(input.RuleType) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid rule type"})
		return
	}

	// Upsert: one rule per (identifier, rule_type) so repeated one-click
	// allowlisting is idempotent rather than creating duplicates.
	result, err := database.DB.Exec(
		`INSERT INTO rules (identifier, policy, rule_type, custom_message, comment, created_by)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(identifier, rule_type) DO UPDATE SET
		   policy = excluded.policy,
		   custom_message = excluded.custom_message,
		   comment = COALESCE(excluded.comment, rules.comment),
		   created_by = excluded.created_by`,
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

// UpdateRule updates a rule's custom_message and/or comment in place (admin only)
func UpdateRule(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid rule ID"})
		return
	}

	var input struct {
		CustomMessage *string `json:"custom_message"`
		Comment       *string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if input.CustomMessage == nil && input.Comment == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "At least one of custom_message or comment is required"})
		return
	}

	var sets []string
	var args []any
	if input.CustomMessage != nil {
		sets = append(sets, "custom_message = ?")
		args = append(args, *input.CustomMessage)
	}
	if input.Comment != nil {
		sets = append(sets, "comment = ?")
		args = append(args, *input.Comment)
	}
	args = append(args, id)

	result, err := database.DB.Exec(
		"UPDATE rules SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...,
	)
	if err != nil {
		log.Printf("Failed to update rule: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update rule"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Rule not found"})
		return
	}

	var r models.Rule
	err = database.DB.QueryRow(
		`SELECT id, identifier, policy, rule_type, custom_message, comment, created_by, proposal_id, created_at
		 FROM rules WHERE id = ?`,
		id,
	).Scan(&r.ID, &r.Identifier, &r.Policy, &r.RuleType, &r.CustomMessage, &r.Comment, &r.CreatedBy, &r.ProposalID, &r.CreatedAt)
	if err != nil {
		log.Printf("Failed to fetch updated rule: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch updated rule"})
		return
	}

	c.JSON(http.StatusOK, r)
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
