package handlers

import (
	"database/sql"
	"encoding/csv"
	"krampus/server/database"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// csvSafe neutralizes spreadsheet formula injection: a leading =, +, -, @,
// tab, or CR makes Excel/Sheets execute the cell. Event fields come from the
// (optionally unauthenticated) Santa upload, so they are attacker-influenced.
func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

// startCSVExport writes the CSV headers and the header row, returning a
// ready-to-use csv.Writer. Call this only after confirming the query
// succeeds, so a query failure can still be reported as JSON 500.
func startCSVExport(c *gin.Context, filename string, header []string) *csv.Writer {
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	w := csv.NewWriter(c.Writer)
	if err := w.Write(header); err != nil {
		log.Printf("Failed to write %s export header: %v", filename, err)
	}
	return w
}

// finishCSVExport flushes the writer and logs (but cannot report via HTTP
// status, since streaming has already begun) any flush error.
func finishCSVExport(filename string, w *csv.Writer) {
	w.Flush()
	if err := w.Error(); err != nil {
		log.Printf("Failed to flush %s export: %v", filename, err)
	}
}

func formatNullTime(t sql.NullTime) string {
	if !t.Valid {
		return ""
	}
	return t.Time.Format(time.RFC3339)
}

// ExportEvents streams all events matching the list filters as CSV.
func ExportEvents(c *gin.Context) {
	p := getPageParams(c)
	where, args, err := buildEventsFilter(c, p)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	query := `SELECT id, machine_id, file_hash, file_path, decision,
	                 executing_user, cert_sha256, cert_cn, bundle_id, bundle_name,
	                 bundle_path, signing_id, team_id, quarantine_data_url,
	                 quarantine_timestamp, execution_time
	          FROM events` + where + " ORDER BY execution_time DESC"

	rows, err := database.DB.Query(query, args...)
	if err != nil {
		log.Printf("Failed to query events for export: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to export events"})
		return
	}
	defer rows.Close()

	w := startCSVExport(c, "events.csv", []string{"id", "machine_id", "file_hash",
		"file_path", "decision", "executing_user", "cert_sha256", "cert_cn",
		"bundle_id", "bundle_name", "bundle_path", "signing_id", "team_id",
		"quarantine_data_url", "quarantine_timestamp", "execution_time"})

	skipped := 0
	for rows.Next() {
		var id int64
		var machineID, fileHash string
		var filePath, decision, executingUser, certSHA256, certCN, bundleID,
			bundleName, bundlePath, signingID, teamID, quarantineDataURL sql.NullString
		var quarantineTimestamp sql.NullTime
		var executionTime time.Time
		if err := rows.Scan(&id, &machineID, &fileHash, &filePath, &decision,
			&executingUser, &certSHA256, &certCN, &bundleID, &bundleName,
			&bundlePath, &signingID, &teamID, &quarantineDataURL,
			&quarantineTimestamp, &executionTime); err != nil {
			log.Printf("Failed to scan event for export: %v", err)
			skipped++
			continue
		}
		record := []string{
			strconv.FormatInt(id, 10), csvSafe(machineID), csvSafe(fileHash),
			csvSafe(filePath.String), csvSafe(decision.String),
			csvSafe(executingUser.String), csvSafe(certSHA256.String),
			csvSafe(certCN.String), csvSafe(bundleID.String),
			csvSafe(bundleName.String), csvSafe(bundlePath.String),
			csvSafe(signingID.String), csvSafe(teamID.String),
			csvSafe(quarantineDataURL.String), formatNullTime(quarantineTimestamp),
			executionTime.Format(time.RFC3339),
		}
		// Streaming has already begun (headers + header row flushed to the
		// client), so a write error here can't be turned into an HTTP error
		// status -- just log it and stop.
		if err := w.Write(record); err != nil {
			log.Printf("Failed to write event export row: %v", err)
			return
		}
	}
	if err := rows.Err(); err != nil {
		log.Printf("Error iterating events for export: %v", err)
	}
	if skipped > 0 {
		log.Printf("events export: skipped %d unreadable rows", skipped)
	}

	finishCSVExport("events.csv", w)
}

// ExportPrograms streams all programs matching the list filters as CSV.
func ExportPrograms(c *gin.Context) {
	p := getPageParams(c)
	where, args := buildProgramsFilter(c, p)

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
	          ORDER BY execution_count DESC`

	rows, err := database.DB.Query(query, args...)
	if err != nil {
		log.Printf("Failed to query programs for export: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to export programs"})
		return
	}
	defer rows.Close()

	w := startCSVExport(c, "programs.csv", []string{"file_hash", "file_path",
		"bundle_id", "bundle_name", "cert_cn", "cert_sha256", "signing_id",
		"team_id", "execution_count", "allow_count", "block_count", "last_seen"})

	skipped := 0
	for rows.Next() {
		var fileHash string
		var filePath, bundleID, bundleName, certCN, certSHA256, signingID,
			teamID sql.NullString
		var executionCount, allowCount, blockCount int
		var lastSeen string
		if err := rows.Scan(&fileHash, &filePath, &bundleID, &bundleName,
			&certCN, &certSHA256, &signingID, &teamID, &executionCount,
			&allowCount, &blockCount, &lastSeen); err != nil {
			log.Printf("Failed to scan program for export: %v", err)
			skipped++
			continue
		}
		record := []string{
			csvSafe(fileHash), csvSafe(filePath.String), csvSafe(bundleID.String),
			csvSafe(bundleName.String), csvSafe(certCN.String),
			csvSafe(certSHA256.String), csvSafe(signingID.String),
			csvSafe(teamID.String), strconv.Itoa(executionCount),
			strconv.Itoa(allowCount), strconv.Itoa(blockCount), lastSeen,
		}
		if err := w.Write(record); err != nil {
			log.Printf("Failed to write program export row: %v", err)
			return
		}
	}
	if err := rows.Err(); err != nil {
		log.Printf("Error iterating programs for export: %v", err)
	}
	if skipped > 0 {
		log.Printf("programs export: skipped %d unreadable rows", skipped)
	}

	finishCSVExport("programs.csv", w)
}

// ExportRules streams all rules matching the list filters as CSV.
func ExportRules(c *gin.Context) {
	p := getPageParams(c)
	where, args := buildRulesFilter(c, p)

	query := `SELECT id, identifier, policy, rule_type, custom_message,
	                 comment, created_at
	          FROM rules` + where + " ORDER BY created_at DESC"

	rows, err := database.DB.Query(query, args...)
	if err != nil {
		log.Printf("Failed to query rules for export: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to export rules"})
		return
	}
	defer rows.Close()

	w := startCSVExport(c, "rules.csv", []string{"id", "identifier", "policy",
		"rule_type", "custom_message", "comment", "created_at"})

	skipped := 0
	for rows.Next() {
		var id int64
		var identifier, policy, ruleType string
		var customMessage, comment sql.NullString
		var createdAt time.Time
		if err := rows.Scan(&id, &identifier, &policy, &ruleType,
			&customMessage, &comment, &createdAt); err != nil {
			log.Printf("Failed to scan rule for export: %v", err)
			skipped++
			continue
		}
		record := []string{
			strconv.FormatInt(id, 10), csvSafe(identifier), policy, ruleType,
			csvSafe(customMessage.String), csvSafe(comment.String),
			createdAt.Format(time.RFC3339),
		}
		if err := w.Write(record); err != nil {
			log.Printf("Failed to write rule export row: %v", err)
			return
		}
	}
	if err := rows.Err(); err != nil {
		log.Printf("Error iterating rules for export: %v", err)
	}
	if skipped > 0 {
		log.Printf("rules export: skipped %d unreadable rows", skipped)
	}

	finishCSVExport("rules.csv", w)
}
