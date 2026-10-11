package handlers

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"

	"krampus/server/config"
	"krampus/server/database"

	"github.com/gin-gonic/gin"
)

type statsTotals struct {
	Machines         int `json:"machines"`
	Rules            int `json:"rules"`
	PendingProposals int `json:"pending_proposals"`
	Events           int `json:"events"`
}

type statsDaily struct {
	Date    string `json:"date"`
	Allowed int    `json:"allowed"`
	Blocked int    `json:"blocked"`
}

type statsTopBlocked struct {
	FileHash   string `json:"file_hash"`
	BundleName string `json:"bundle_name"`
	FilePath   string `json:"file_path"`
	Count      int    `json:"count"`
}

type statsMachineActivity struct {
	MachineID string `json:"machine_id"`
	Hostname  string `json:"hostname"`
	Events    int    `json:"events"`
	LastSync  string `json:"last_sync"`
}

type statsStaleMachine struct {
	MachineID string `json:"machine_id"`
	Hostname  string `json:"hostname"`
	LastSync  string `json:"last_sync"`
}

// StatsOverview returns aggregate counts, a thirty-day allow/block trend,
// top blocked binaries, per-machine activity, and stale-machine detection
// for the dashboard.
func StatsOverview(c *gin.Context) {
	totals, err := fetchStatsTotals()
	if err != nil {
		log.Printf("stats: totals: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch stats"})
		return
	}

	daily, err := fetchStatsDaily()
	if err != nil {
		log.Printf("stats: daily: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch stats"})
		return
	}

	topBlocked, err := fetchStatsTopBlocked()
	if err != nil {
		log.Printf("stats: top blocked: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch stats"})
		return
	}

	machineActivity, err := fetchStatsMachineActivity()
	if err != nil {
		log.Printf("stats: machine activity: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch stats"})
		return
	}

	staleMachines, err := fetchStatsStaleMachines()
	if err != nil {
		log.Printf("stats: stale machines: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch stats"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"totals":            totals,
		"daily":             daily,
		"top_blocked":       topBlocked,
		"machine_activity":  machineActivity,
		"stale_machines":    staleMachines,
		"stale_after_hours": config.AppConfig.StaleMachineHours,
	})
}

func fetchStatsTotals() (statsTotals, error) {
	var t statsTotals
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM machines`).Scan(&t.Machines); err != nil {
		return t, err
	}
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM rules`).Scan(&t.Rules); err != nil {
		return t, err
	}
	if err := database.DB.QueryRow(
		`SELECT COUNT(*) FROM proposals WHERE status = 'PENDING'`,
	).Scan(&t.PendingProposals); err != nil {
		return t, err
	}
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM events`).Scan(&t.Events); err != nil {
		return t, err
	}
	return t, nil
}

// fetchStatsDaily buckets by UTC day, so events executed near local
// midnight may land on the adjacent day.
func fetchStatsDaily() ([]statsDaily, error) {
	rows, err := database.DB.Query(
		`SELECT date(execution_time) d,
		        SUM(CASE WHEN decision GLOB 'ALLOW*' THEN 1 ELSE 0 END) allowed,
		        SUM(CASE WHEN decision GLOB 'BLOCK*' THEN 1 ELSE 0 END) blocked
		 FROM events
		 WHERE execution_time >= datetime('now', '-30 days')
		 GROUP BY d
		 ORDER BY d`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	daily := []statsDaily{}
	for rows.Next() {
		var d statsDaily
		if err := rows.Scan(&d.Date, &d.Allowed, &d.Blocked); err != nil {
			return nil, err
		}
		daily = append(daily, d)
	}
	return daily, rows.Err()
}

// fetchStatsTopBlocked uses GLOB rather than LIKE: GLOB is case-sensitive,
// which lets SQLite drive the filter through idx_events_decision.
func fetchStatsTopBlocked() ([]statsTopBlocked, error) {
	rows, err := database.DB.Query(
		`SELECT file_hash, COALESCE(MAX(bundle_name), ''), COALESCE(MAX(file_path), ''), COUNT(*) c
		 FROM events
		 WHERE decision GLOB 'BLOCK*'
		 GROUP BY file_hash
		 ORDER BY c DESC
		 LIMIT 10`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	topBlocked := []statsTopBlocked{}
	for rows.Next() {
		var tb statsTopBlocked
		if err := rows.Scan(&tb.FileHash, &tb.BundleName, &tb.FilePath, &tb.Count); err != nil {
			return nil, err
		}
		topBlocked = append(topBlocked, tb)
	}
	return topBlocked, rows.Err()
}

// fetchStatsMachineActivity (like the totals) does a full event-table pass
// on every load. Acceptable at current scale; move to summary counters if
// the events table grows large.
func fetchStatsMachineActivity() ([]statsMachineActivity, error) {
	rows, err := database.DB.Query(
		`SELECT m.machine_id, COALESCE(m.hostname, ''), COUNT(e.id), COALESCE(m.last_sync, '')
		 FROM machines m
		 LEFT JOIN events e ON e.machine_id = m.machine_id
		 GROUP BY m.machine_id
		 ORDER BY COUNT(e.id) DESC, m.machine_id
		 LIMIT 10`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	activity := []statsMachineActivity{}
	for rows.Next() {
		var a statsMachineActivity
		if err := rows.Scan(&a.MachineID, &a.Hostname, &a.Events, &a.LastSync); err != nil {
			return nil, err
		}
		activity = append(activity, a)
	}
	return activity, rows.Err()
}

func fetchStatsStaleMachines() ([]statsStaleMachine, error) {
	cutoff := fmt.Sprintf("-%d hours", config.AppConfig.StaleMachineHours)
	rows, err := database.DB.Query(
		`SELECT machine_id, COALESCE(hostname, ''), last_sync
		 FROM machines
		 WHERE last_sync IS NULL OR last_sync < datetime('now', ?)`,
		cutoff,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stale := []statsStaleMachine{}
	for rows.Next() {
		var s statsStaleMachine
		var lastSync sql.NullString
		if err := rows.Scan(&s.MachineID, &s.Hostname, &lastSync); err != nil {
			return nil, err
		}
		s.LastSync = lastSync.String
		stale = append(stale, s)
	}
	return stale, rows.Err()
}
