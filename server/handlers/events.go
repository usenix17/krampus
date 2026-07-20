package handlers

import (
	"krampus/server/database"
	"krampus/server/models"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ListEvents returns all execution events with pagination
func ListEvents(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset := (page - 1) * limit

	// Optional filters
	machineID := c.Query("machine_id")
	decision := c.Query("decision")

	query := `SELECT id, machine_id, file_hash, file_path, decision,
	                 executing_user, cert_sha256, cert_cn, bundle_id, bundle_name,
	                 bundle_path, signing_id, team_id, quarantine_data_url,
	                 quarantine_timestamp, execution_time
	          FROM events WHERE 1=1`
	args := []interface{}{}

	if machineID != "" {
		query += " AND machine_id = ?"
		args = append(args, machineID)
	}
	if decision != "" {
		query += " AND decision = ?"
		args = append(args, decision)
	}

	query += " ORDER BY execution_time DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := database.DB.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch events"})
		return
	}
	defer rows.Close()

	events := []models.Event{}
	for rows.Next() {
		var event models.Event
		err := rows.Scan(
			&event.ID, &event.MachineID, &event.FileHash, &event.FilePath,
			&event.Decision, &event.ExecutingUser, &event.CertSHA256,
			&event.CertCN, &event.BundleID, &event.BundleName, &event.BundlePath,
			&event.SigningID, &event.TeamID, &event.QuarantineDataURL,
			&event.QuarantineTimestamp, &event.ExecutionTime,
		)
		if err != nil {
			continue
		}
		events = append(events, event)
	}

	// Get total count
	countQuery := "SELECT COUNT(*) FROM events WHERE 1=1"
	countArgs := []interface{}{}
	if machineID != "" {
		countQuery += " AND machine_id = ?"
		countArgs = append(countArgs, machineID)
	}
	if decision != "" {
		countQuery += " AND decision = ?"
		countArgs = append(countArgs, decision)
	}

	var total int
	database.DB.QueryRow(countQuery, countArgs...).Scan(&total)

	c.JSON(http.StatusOK, gin.H{
		"events": events,
		"total":  total,
		"page":   page,
		"limit":  limit,
	})
}

// ListPrograms returns unique programs/binaries seen in events
func ListPrograms(c *gin.Context) {
	query := `SELECT
	            file_hash,
	            file_path,
	            bundle_id,
	            bundle_name,
	            cert_cn,
	            cert_sha256,
	            signing_id,
	            team_id,
	            COUNT(*) as execution_count,
	            SUM(CASE WHEN decision = 'ALLOW' THEN 1 ELSE 0 END) as allow_count,
	            SUM(CASE WHEN decision = 'BLOCK' THEN 1 ELSE 0 END) as block_count,
	            MAX(execution_time) as last_seen
	          FROM events
	          GROUP BY file_hash
	          ORDER BY execution_count DESC
	          LIMIT 100`

	rows, err := database.DB.Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch programs"})
		return
	}
	defer rows.Close()

	type Program struct {
		FileHash       string  `json:"file_hash"`
		FilePath       *string `json:"file_path"`
		BundleID       *string `json:"bundle_id"`
		BundleName     *string `json:"bundle_name"`
		CertCN         *string `json:"cert_cn"`
		CertSHA256     *string `json:"cert_sha256"`
		SigningID      *string `json:"signing_id"`
		TeamID         *string `json:"team_id"`
		ExecutionCount int     `json:"execution_count"`
		AllowCount     int     `json:"allow_count"`
		BlockCount     int     `json:"block_count"`
		LastSeen       string  `json:"last_seen"`
	}

	programs := []Program{}
	for rows.Next() {
		var p Program
		err := rows.Scan(
			&p.FileHash, &p.FilePath, &p.BundleID,
			&p.BundleName, &p.CertCN, &p.CertSHA256, &p.SigningID, &p.TeamID,
			&p.ExecutionCount, &p.AllowCount, &p.BlockCount, &p.LastSeen,
		)
		if err != nil {
			continue
		}
		programs = append(programs, p)
	}

	c.JSON(http.StatusOK, gin.H{
		"programs": programs,
		"total":    len(programs),
	})
}
