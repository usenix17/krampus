package handlers

import (
	"errors"
	"krampus/server/database"
	"krampus/server/models"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// buildEventsFilter assembles the WHERE clause and bound args shared by the
// events list count and page queries. It covers the machine_id and decision
// exact filters, an inclusive from/to date range on execution_time, and the
// free-text search. Date bounds must be YYYY-MM-DD (anything else errors);
// `to` is made inclusive via `< date(?, '+1 day')`. The returned clause
// includes the leading " WHERE " or is empty.
func buildEventsFilter(c *gin.Context, p pageParams) (string, []any, error) {
	var conds []string
	var args []any

	if machineID := c.Query("machine_id"); machineID != "" {
		conds = append(conds, "machine_id = ?")
		args = append(args, machineID)
	}
	if decision := c.Query("decision"); decision != "" {
		conds = append(conds, "decision = ?")
		args = append(args, decision)
	}
	if from := c.Query("from"); from != "" {
		if _, err := time.Parse("2006-01-02", from); err != nil {
			return "", nil, errors.New("invalid date, want YYYY-MM-DD")
		}
		conds = append(conds, "execution_time >= ?")
		args = append(args, from)
	}
	if to := c.Query("to"); to != "" {
		if _, err := time.Parse("2006-01-02", to); err != nil {
			return "", nil, errors.New("invalid date, want YYYY-MM-DD")
		}
		conds = append(conds, "execution_time < date(?, '+1 day')")
		args = append(args, to)
	}
	if clause, sargs := searchClause(p.Search,
		"file_path", "file_hash", "bundle_name", "executing_user"); clause != "" {
		conds = append(conds, clause)
		args = append(args, sargs...)
	}

	if len(conds) == 0 {
		return "", args, nil
	}
	return " WHERE " + strings.Join(conds, " AND "), args, nil
}

// ListEvents returns a paginated, searchable, filterable page of events.
func ListEvents(c *gin.Context) {
	p := getPageParams(c)
	where, args, err := buildEventsFilter(c, p)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var total int
	if err := database.DB.QueryRow(
		"SELECT COUNT(*) FROM events"+where, args...,
	).Scan(&total); err != nil {
		log.Printf("Failed to count events: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch events"})
		return
	}

	query := `SELECT id, machine_id, file_hash, file_path, decision,
	                 executing_user, cert_sha256, cert_cn, bundle_id, bundle_name,
	                 bundle_path, signing_id, team_id, quarantine_data_url,
	                 quarantine_timestamp, execution_time
	          FROM events` + where +
		" ORDER BY execution_time DESC LIMIT ? OFFSET ?"
	pageArgs := append(append([]any{}, args...), p.PerPage, p.Offset)

	rows, err := database.DB.Query(query, pageArgs...)
	if err != nil {
		log.Printf("Failed to query events: %v", err)
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

	c.JSON(http.StatusOK, pageResponse{
		Items: events, Total: total, Page: p.Page, PerPage: p.PerPage,
	})
}

// buildProgramsFilter assembles the WHERE clause and bound args applied to
// events before they are grouped by file_hash. Only free-text search applies.
// The search gates group membership through a file_hash subquery so a program
// whose rows only partially match still aggregates over ALL of its events;
// filtering rows directly would under-count execution/allow/block totals.
func buildProgramsFilter(_ *gin.Context, p pageParams) (string, []any) {
	clause, args := searchClause(p.Search,
		"file_hash", "bundle_name", "file_path", "signing_id")
	if clause == "" {
		return "", args
	}
	return " WHERE file_hash IN (SELECT file_hash FROM events WHERE " +
		clause + ")", args
}

// ListPrograms returns a paginated, searchable page of unique programs
// (binaries) aggregated from events, grouped by file_hash.
//
// Note: every page re-aggregates the whole events table (minus the search
// gate). Acceptable at current scale; move to a summary table if it grows.
func ListPrograms(c *gin.Context) {
	p := getPageParams(c)
	where, args := buildProgramsFilter(c, p)

	var total int
	if err := database.DB.QueryRow(
		"SELECT COUNT(DISTINCT file_hash) FROM events"+where, args...,
	).Scan(&total); err != nil {
		log.Printf("Failed to count programs: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch programs"})
		return
	}

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
	          FROM events` + where + `
	          GROUP BY file_hash
	          ORDER BY execution_count DESC
	          LIMIT ? OFFSET ?`
	pageArgs := append(append([]any{}, args...), p.PerPage, p.Offset)

	rows, err := database.DB.Query(query, pageArgs...)
	if err != nil {
		log.Printf("Failed to query programs: %v", err)
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
		var pr Program
		err := rows.Scan(
			&pr.FileHash, &pr.FilePath, &pr.BundleID,
			&pr.BundleName, &pr.CertCN, &pr.CertSHA256, &pr.SigningID, &pr.TeamID,
			&pr.ExecutionCount, &pr.AllowCount, &pr.BlockCount, &pr.LastSeen,
		)
		if err != nil {
			continue
		}
		programs = append(programs, pr)
	}

	c.JSON(http.StatusOK, pageResponse{
		Items: programs, Total: total, Page: p.Page, PerPage: p.PerPage,
	})
}
