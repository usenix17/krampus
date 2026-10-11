package handlers

import (
	"database/sql"
	"krampus/server/database"
	"krampus/server/models"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// buildUsersFilter assembles the WHERE clause and bound args shared by the
// users list count and page queries from the search param.
func buildUsersFilter(_ *gin.Context, p pageParams) (string, []any) {
	clause, args := searchClause(p.Search, "username", "email")
	if clause == "" {
		return "", args
	}
	return " WHERE " + clause, args
}

// ListUsers returns a paginated, searchable page of users (admin only).
func ListUsers(c *gin.Context) {
	p := getPageParams(c)
	where, args := buildUsersFilter(c, p)

	var total int
	if err := database.DB.QueryRow(
		"SELECT COUNT(*) FROM users"+where, args...,
	).Scan(&total); err != nil {
		log.Printf("Failed to count users: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch users"})
		return
	}

	query := `SELECT id, username, role, oidc_subject, email, created_at, last_login
	          FROM users` + where + " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	pageArgs := append(append([]any{}, args...), p.PerPage, p.Offset)

	rows, err := database.DB.Query(query, pageArgs...)
	if err != nil {
		log.Printf("Failed to query users: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch users"})
		return
	}
	defer rows.Close()

	users := []models.User{}
	for rows.Next() {
		var u models.User
		err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.OIDCSubject, &u.Email, &u.CreatedAt, &u.LastLogin)
		if err != nil {
			log.Printf("Failed to scan user: %v", err)
			continue
		}
		users = append(users, u)
	}

	c.JSON(http.StatusOK, pageResponse{
		Items: users, Total: total, Page: p.Page, PerPage: p.PerPage,
	})
}

// GetUser returns a single user by ID (admin only)
func GetUser(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	var u models.User
	err = database.DB.QueryRow(
		`SELECT id, username, role, oidc_subject, email, created_at, last_login
		 FROM users WHERE id = ?`,
		id,
	).Scan(&u.ID, &u.Username, &u.Role, &u.OIDCSubject, &u.Email, &u.CreatedAt, &u.LastLogin)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	if err != nil {
		log.Printf("Failed to fetch user: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch user"})
		return
	}

	c.JSON(http.StatusOK, u)
}

// UpdateUser updates a user's role (admin only)
func UpdateUser(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	var input struct {
		Role string `json:"role" binding:"required"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate role
	if input.Role != string(models.RoleAdmin) && input.Role != string(models.RoleUser) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role"})
		return
	}

	_, err = database.DB.Exec(`UPDATE users SET role = ? WHERE id = ?`, input.Role, id)
	if err != nil {
		log.Printf("Failed to update user: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User updated successfully"})
}

// DeleteUser deletes a user (admin only)
func DeleteUser(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	result, err := database.DB.Exec(`DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		log.Printf("Failed to delete user: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete user"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User deleted successfully"})
}
