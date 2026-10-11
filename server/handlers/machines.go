package handlers

import (
	"database/sql"
	"fmt"
	"krampus/server/config"
	"krampus/server/database"
	"krampus/server/models"
	"krampus/server/services"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ListMachines returns all enrolled machines
func ListMachines(c *gin.Context) {
	rows, err := database.DB.Query(
		`SELECT id, machine_id, serial_number, hostname, os_version, os_build,
		        santa_version, client_mode, enrolled_at, last_sync, last_preflight_sync
		 FROM machines ORDER BY enrolled_at DESC`,
	)
	if err != nil {
		log.Printf("Failed to query machines: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch machines"})
		return
	}
	defer rows.Close()

	machines := []models.Machine{}
	for rows.Next() {
		var m models.Machine
		err := rows.Scan(
			&m.ID, &m.MachineID, &m.SerialNumber, &m.Hostname, &m.OSVersion, &m.OSBuild,
			&m.SantaVersion, &m.ClientMode, &m.EnrolledAt, &m.LastSync, &m.LastPreflightSync,
		)
		if err != nil {
			log.Printf("Failed to scan machine: %v", err)
			continue
		}
		machines = append(machines, m)
	}

	c.JSON(http.StatusOK, machines)
}

// GetMachine returns a single machine by ID
func GetMachine(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid machine ID"})
		return
	}

	var m models.Machine
	err = database.DB.QueryRow(
		`SELECT id, machine_id, serial_number, hostname, os_version, os_build,
		        santa_version, client_mode, enrolled_at, last_sync, last_preflight_sync
		 FROM machines WHERE id = ?`,
		id,
	).Scan(
		&m.ID, &m.MachineID, &m.SerialNumber, &m.Hostname, &m.OSVersion, &m.OSBuild,
		&m.SantaVersion, &m.ClientMode, &m.EnrolledAt, &m.LastSync, &m.LastPreflightSync,
	)

	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Machine not found"})
		return
	}
	if err != nil {
		log.Printf("Failed to fetch machine: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch machine"})
		return
	}

	c.JSON(http.StatusOK, m)
}

// ListMachineEvents returns a paginated page of a single machine's events,
// using the same :id semantics as GetMachine (the numeric machines.id).
func ListMachineEvents(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid machine ID"})
		return
	}

	var machineID string
	err = database.DB.QueryRow(`SELECT machine_id FROM machines WHERE id = ?`, id).Scan(&machineID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Machine not found"})
		return
	}
	if err != nil {
		log.Printf("Failed to fetch machine: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch machine"})
		return
	}

	p := getPageParams(c)

	var total int
	if err := database.DB.QueryRow(
		`SELECT COUNT(*) FROM events WHERE machine_id = ?`, machineID,
	).Scan(&total); err != nil {
		log.Printf("Failed to count machine events: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch events"})
		return
	}

	rows, err := database.DB.Query(
		`SELECT id, machine_id, file_hash, file_path, decision,
		        executing_user, cert_sha256, cert_cn, bundle_id, bundle_name,
		        bundle_path, signing_id, team_id, quarantine_data_url,
		        quarantine_timestamp, execution_time
		 FROM events WHERE machine_id = ?
		 ORDER BY execution_time DESC LIMIT ? OFFSET ?`,
		machineID, p.PerPage, p.Offset,
	)
	if err != nil {
		log.Printf("Failed to query machine events: %v", err)
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
			log.Printf("Failed to scan machine event: %v", err)
			continue
		}
		events = append(events, event)
	}

	c.JSON(http.StatusOK, pageResponse{
		Items: events, Total: total, Page: p.Page, PerPage: p.PerPage,
	})
}

// RegisterMachine registers a new machine
func RegisterMachine(c *gin.Context) {
	var input struct {
		MachineID    string  `json:"machine_id" binding:"required"`
		SerialNumber *string `json:"serial_number"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Insert machine
	_, err := database.DB.Exec(
		`INSERT INTO machines (machine_id, serial_number) VALUES (?, ?)
		 ON CONFLICT(machine_id) DO NOTHING`,
		input.MachineID, input.SerialNumber,
	)
	if err != nil {
		log.Printf("Failed to register machine: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register machine"})
		return
	}

	// LastInsertId is 0 when ON CONFLICT skips the insert, so read the real id.
	var machineID int64
	err = database.DB.QueryRow(
		`SELECT id FROM machines WHERE machine_id = ?`, input.MachineID,
	).Scan(&machineID)
	if err != nil {
		log.Printf("Failed to look up machine id: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register machine"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":         machineID,
		"machine_id": input.MachineID,
		"message":    "Machine registered successfully",
	})
}

// GenerateMobileConfig generates and downloads a mobileconfig configuration profile
func GenerateMobileConfig(c *gin.Context) {
	machineID := c.Param("id")

	var input struct {
		ClientMode       string `json:"client_mode" binding:"required"`
		UploadInterval   int    `json:"upload_interval"`
		OrganizationName string `json:"organization_name"`
		MachineOwner     string `json:"machine_owner"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate client mode
	if input.ClientMode != "MONITOR" && input.ClientMode != "LOCKDOWN" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid client mode. Must be MONITOR or LOCKDOWN"})
		return
	}

	// Default upload interval if not specified
	if input.UploadInterval == 0 {
		input.UploadInterval = 600 // 10 minutes
	}

	// Generate mobileconfig
	mobileconfig := services.GenerateMobileConfig(
		machineID,
		input.ClientMode,
		config.AppConfig.SyncBaseURL,
		input.OrganizationName,
		input.MachineOwner,
		input.UploadInterval,
	)

	// Set headers for file download
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s.mobileconfig", machineID))
	c.Data(http.StatusOK, "application/x-apple-aspen-config", []byte(mobileconfig))
}

// DeleteMachine deletes a machine (admin only)
func DeleteMachine(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid machine ID"})
		return
	}

	result, err := database.DB.Exec(`DELETE FROM machines WHERE id = ?`, id)
	if err != nil {
		log.Printf("Failed to delete machine: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete machine"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Machine not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Machine deleted successfully"})
}
