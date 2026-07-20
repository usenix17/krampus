package handlers

import (
	"krampus/server/config"
	"krampus/server/database"
	"krampus/server/models"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Preflight handles the Santa preflight request
func Preflight(c *gin.Context) {
	machineID := c.Param("machine_id")

	// Santa can send either JSON or URL-encoded forms
	// client_mode can be sent as integer (1=MONITOR, 2=LOCKDOWN) or string ("MONITOR", "LOCKDOWN")
	var input struct {
		Hostname         string `json:"primary_user" form:"primary_user"`
		OSVersion        string `json:"os_version" form:"os_version"`
		OSBuild          string `json:"os_build" form:"os_build"`
		SantaVersion     string `json:"santa_version" form:"santa_version"`
		SerialNumber     string `json:"serial_num" form:"serial_num"`
		ClientModeInt    int    `json:"-" form:"client_mode"`
		ClientModeString string `json:"client_mode" form:"-"`
		ModelID          string `json:"model_identifier" form:"model_identifier"`
	}

	// Log the raw request for debugging
	contentType := c.GetHeader("Content-Type")
	contentEncoding := c.GetHeader("Content-Encoding")
	log.Printf("Preflight request from %s - Content-Type: %s, Content-Encoding: %s",
		machineID, contentType, contentEncoding)

	// Santa can send either JSON or form-encoded data
	// Try JSON first (modern Santa), then fall back to form data
	var err error
	if strings.Contains(contentType, "json") {
		err = c.ShouldBindJSON(&input)
	} else {
		err = c.ShouldBind(&input)
	}

	if err != nil {
		log.Printf("Preflight parse error for machine %s: %v", machineID, err)
		// Continue anyway - Santa might send minimal data
	}

	// Convert client mode to standard format for database
	clientMode := ""
	if input.ClientModeString != "" {
		// Santa sent string format
		clientMode = strings.ToUpper(input.ClientModeString)
	} else if input.ClientModeInt == 1 {
		clientMode = "MONITOR"
	} else if input.ClientModeInt == 2 {
		clientMode = "LOCKDOWN"
	}

	log.Printf("Parsed preflight data: hostname=%s, os_version=%s, santa_version=%s, client_mode=%s",
		input.Hostname, input.OSVersion, input.SantaVersion, clientMode)

	// Only update machine if we have meaningful data OR just update the preflight timestamp
	if clientMode == "" && input.SerialNumber == "" && input.Hostname == "" {
		// Just update the preflight timestamp
		_, err := database.DB.Exec(
			`INSERT INTO machines (machine_id, last_preflight_sync)
			 VALUES (?, datetime('now'))
			 ON CONFLICT(machine_id) DO UPDATE SET
			   last_preflight_sync = datetime('now')`,
			machineID,
		)
		if err != nil {
			log.Printf("Failed to update preflight timestamp: %v", err)
		}
	} else {
		// Update or insert machine information
		_, err := database.DB.Exec(
			`INSERT INTO machines (machine_id, serial_number, hostname, os_version, os_build, santa_version, client_mode, last_preflight_sync)
			 VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'))
			 ON CONFLICT(machine_id) DO UPDATE SET
			   serial_number = COALESCE(NULLIF(excluded.serial_number, ''), serial_number),
			   hostname = COALESCE(NULLIF(excluded.hostname, ''), hostname),
			   os_version = COALESCE(NULLIF(excluded.os_version, ''), os_version),
			   os_build = COALESCE(NULLIF(excluded.os_build, ''), os_build),
			   santa_version = COALESCE(NULLIF(excluded.santa_version, ''), santa_version),
			   client_mode = CASE WHEN excluded.client_mode != '' THEN excluded.client_mode ELSE client_mode END,
			   last_preflight_sync = datetime('now')`,
			machineID, input.SerialNumber, input.Hostname, input.OSVersion,
			input.OSBuild, input.SantaVersion, clientMode,
		)
		if err != nil {
			log.Printf("Failed to update machine: %v (clientMode=%s)", err, clientMode)
			// Don't return error - just log it and continue
		}
	}

	// Return sync configuration. The desired client mode is operator-controlled
	// via SANTA_CLIENT_MODE rather than hardcoded.
	c.JSON(http.StatusOK, gin.H{
		"client_mode":             config.AppConfig.SantaClientMode,
		"batch_size":              100,
		"upload_logs_url":         "",
		"clean_sync":              false,
		"enable_bundles":          true,
		"enable_transitive_rules": false,
		"blocked_path_regex":      "",
		"allowed_path_regex":      "",
		"enable_all_event_upload": false,
	})
}

// EventUpload handles Santa event upload (supports both JSON and protobuf)
func EventUpload(c *gin.Context) {
	machineID := c.Param("machine_id")

	// Try JSON first
	var input struct {
		Events []models.SantaEvent `json:"events"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		// Non-JSON body is almost certainly protobuf. Decoding protobuf event
		// uploads is not yet implemented, so these events are acknowledged but
		// not stored. Reconfigure Santa with EventLogType=json (redownload the
		// mobileconfig from the Krampus UI) to capture events until then.
		log.Printf("WARNING: EventUpload from %s is non-JSON (Content-Type=%s, Content-Encoding=%s); "+
			"protobuf decoding is not implemented, events not stored. Set EventLogType=json in Santa.",
			machineID, c.GetHeader("Content-Type"), c.GetHeader("Content-Encoding"))

		// Return success anyway - Santa needs a 200 response to continue syncing
		c.JSON(http.StatusOK, gin.H{
			"event_upload_bundle_binaries": []string{},
		})
		return
	}

	log.Printf("EventUpload: Received %d JSON events from %s", len(input.Events), machineID)

	// Insert JSON events into database
	eventCount := 0
	for _, event := range input.Events {
		execTime := time.Unix(int64(event.ExecutionTime), 0)

		_, err := database.DB.Exec(
			`INSERT INTO events (machine_id, file_path, file_hash, execution_time, decision,
			                     executing_user, cert_sha256, cert_cn, bundle_id, bundle_name,
			                     bundle_path, signing_id, team_id, quarantine_data_url)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			machineID, event.FilePath, event.FileSHA256, execTime, event.Decision,
			event.ExecutingUser, event.CertificateSHA256, event.CertificateCN,
			event.BundleID, event.BundleName, event.BundlePath,
			event.SigningID, event.TeamID, event.QuarantineDataURL,
		)
		if err != nil {
			log.Printf("Failed to insert event: %v", err)
			continue
		}
		eventCount++
	}

	log.Printf("EventUpload: Successfully stored %d events from %s", eventCount, machineID)

	// Return acknowledgement
	c.JSON(http.StatusOK, gin.H{
		"event_upload_bundle_binaries": []string{},
	})
}

// RuleDownload handles Santa rule download requests
func RuleDownload(c *gin.Context) {
	machineID := c.Param("machine_id")

	var input struct {
		Cursor string `json:"cursor"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		// Cursor is optional, ignore error
	}

	// The cursor is the id of the last rule sent in the previous batch. It is
	// emitted (and echoed back) as a string so it round-trips cleanly.
	var startID int64 = 0
	if input.Cursor != "" {
		parsed, err := strconv.ParseInt(input.Cursor, 10, 64)
		if err != nil {
			log.Printf("RuleDownload: invalid cursor %q from %s, starting from 0", input.Cursor, machineID)
		} else {
			startID = parsed
		}
	}

	// Fetch rules with pagination
	batchSize := 100
	rows, err := database.DB.Query(
		`SELECT id, identifier, policy, rule_type, custom_message
		 FROM rules
		 WHERE id > ?
		 ORDER BY id
		 LIMIT ?`,
		startID, batchSize,
	)
	if err != nil {
		log.Printf("Failed to query rules: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch rules"})
		return
	}
	defer rows.Close()

	santaRules := []models.SantaRule{}
	var lastID int64

	for rows.Next() {
		var id int64
		var r models.SantaRule
		err := rows.Scan(&id, &r.Identifier, &r.Policy, &r.RuleType, &r.CustomMsg)
		if err != nil {
			log.Printf("Failed to scan rule: %v", err)
			continue
		}
		santaRules = append(santaRules, r)
		lastID = id
	}

	// Prepare response
	response := gin.H{
		"rules": santaRules,
	}

	// If we got a full batch, there might be more rules. Emit the cursor as a
	// string so it matches the type we parse on the way back in.
	if len(santaRules) == batchSize {
		response["cursor"] = strconv.FormatInt(lastID, 10)
	}

	// Update last sync time
	_, _ = database.DB.Exec(
		`UPDATE machines SET last_sync = datetime('now') WHERE machine_id = ?`,
		machineID,
	)

	c.JSON(http.StatusOK, response)
}

// Postflight handles Santa postflight requests
func Postflight(c *gin.Context) {
	machineID := c.Param("machine_id")

	// Log the completion of sync
	_, err := database.DB.Exec(
		`UPDATE machines SET last_sync = datetime('now') WHERE machine_id = ?`,
		machineID,
	)
	if err != nil {
		log.Printf("Failed to update machine last_sync: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{})
}
