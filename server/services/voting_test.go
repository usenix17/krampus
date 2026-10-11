package services

import (
	"database/sql"
	"path/filepath"
	"testing"

	"krampus/server/config"
	"krampus/server/database"
)

func setupVotingTest(t *testing.T, threshold int) {
	t.Helper()
	config.AppConfig = &config.Config{VoteThreshold: threshold}
	if err := database.Initialize(filepath.Join(t.TempDir(), "test.db")); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	mustExec(t, `INSERT INTO users (id, username, role) VALUES (1,'alice','USER'),(2,'bob','USER')`)
	mustExec(t, `INSERT INTO proposals (id, identifier, rule_type, proposed_policy, custom_message, created_by)
		VALUES (1,'ABCDEF','BINARY','ALLOWLIST','Approved by team vote',1)`)
}

func mustExec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := database.DB.Exec(q, args...); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

func mustScan(t *testing.T, q string, dest ...any) {
	t.Helper()
	if err := database.DB.QueryRow(q).Scan(dest...); err != nil {
		t.Fatalf("query %q: %v", q, err)
	}
}

func TestFinalizeWritesCustomMessageForSanta(t *testing.T) {
	setupVotingTest(t, 3)
	if err := FinalizeProposal(1, "ALLOWLIST"); err != nil {
		t.Fatalf("FinalizeProposal: %v", err)
	}
	var msg, comment sql.NullString
	if err := database.DB.QueryRow(
		`SELECT custom_message, comment FROM rules WHERE identifier='ABCDEF' AND rule_type='BINARY'`,
	).Scan(&msg, &comment); err != nil {
		t.Fatalf("rule not created: %v", err)
	}
	if !msg.Valid || msg.String != "Approved by team vote" {
		t.Errorf("custom_message=%v, want proposal message", msg)
	}
	if !comment.Valid || comment.String != "Approved by team vote" {
		t.Errorf("comment=%v, want proposal message", comment)
	}
}

func TestReapprovalPreservesMessageViaCoalesce(t *testing.T) {
	setupVotingTest(t, 3)
	if err := FinalizeProposal(1, "ALLOWLIST"); err != nil {
		t.Fatalf("FinalizeProposal: %v", err)
	}
	mustExec(t, `INSERT INTO proposals (id, identifier, rule_type, proposed_policy, custom_message, created_by)
		VALUES (2,'ABCDEF','BINARY','BLOCKLIST',NULL,2)`)
	if err := FinalizeProposal(2, "BLOCKLIST"); err != nil {
		t.Fatalf("FinalizeProposal re-approval: %v", err)
	}
	var msg sql.NullString
	var policy string
	mustScan(t, `SELECT custom_message, policy FROM rules WHERE identifier='ABCDEF' AND rule_type='BINARY'`,
		&msg, &policy)
	if !msg.Valid || msg.String != "Approved by team vote" {
		t.Errorf("custom_message=%v, want original message preserved", msg)
	}
	if policy != "BLOCKLIST" {
		t.Errorf("policy=%q, want BLOCKLIST", policy)
	}
	var n int
	mustScan(t, `SELECT COUNT(*) FROM rules WHERE identifier='ABCDEF'`, &n)
	if n != 1 {
		t.Errorf("rules=%d, want 1", n)
	}
}

func TestVoteChangeFlipsCount(t *testing.T) {
	setupVotingTest(t, 3)
	if err := SubmitVote(1, 1, "ALLOWLIST"); err != nil {
		t.Fatal(err)
	}
	if err := SubmitVote(1, 1, "BLOCKLIST"); err != nil {
		t.Fatal(err)
	}
	var allow, block int
	mustScan(t, `SELECT allowlist_votes, blocklist_votes FROM proposals WHERE id=1`, &allow, &block)
	if allow != 0 || block != 1 {
		t.Fatalf("allowlist_votes=%d blocklist_votes=%d, want 0 and 1", allow, block)
	}
}

func TestAdminApproveProposal(t *testing.T) {
	setupVotingTest(t, 3)
	if err := AdminApproveProposal(1, "ALLOWLIST"); err != nil {
		t.Fatalf("AdminApproveProposal: %v", err)
	}
	var status string
	mustScan(t, `SELECT status FROM proposals WHERE id=1`, &status)
	if status != "APPROVED" {
		t.Fatalf("status=%s, want APPROVED", status)
	}
	var n int
	mustScan(t, `SELECT COUNT(*) FROM rules WHERE identifier='ABCDEF' AND rule_type='BINARY'`, &n)
	if n != 1 {
		t.Fatalf("rules=%d, want 1", n)
	}
}

func TestVoteThresholdAutoFinalizes(t *testing.T) {
	setupVotingTest(t, 2)
	if err := SubmitVote(1, 1, "ALLOWLIST"); err != nil {
		t.Fatal(err)
	}
	var status string
	mustScan(t, `SELECT status FROM proposals WHERE id=1`, &status)
	if status != "PENDING" {
		t.Fatalf("finalized after 1 of 2 votes (status=%s)", status)
	}
	if err := SubmitVote(2, 1, "ALLOWLIST"); err != nil {
		t.Fatal(err)
	}
	mustScan(t, `SELECT status FROM proposals WHERE id=1`, &status)
	if status != "APPROVED" {
		t.Fatalf("status=%s, want APPROVED", status)
	}
	var n int
	mustScan(t, `SELECT COUNT(*) FROM rules WHERE identifier='ABCDEF'`, &n)
	if n != 1 {
		t.Fatalf("rules=%d, want 1", n)
	}
}

func TestBlocklistThresholdCreatesBlockRule(t *testing.T) {
	setupVotingTest(t, 1)
	if err := SubmitVote(1, 1, "BLOCKLIST"); err != nil {
		t.Fatal(err)
	}
	var policy string
	mustScan(t, `SELECT policy FROM rules WHERE identifier='ABCDEF'`, &policy)
	if policy != "BLOCKLIST" {
		t.Fatalf("policy=%q, want BLOCKLIST", policy)
	}
}

func TestCannotVoteOnFinalizedProposal(t *testing.T) {
	setupVotingTest(t, 1)
	if err := SubmitVote(1, 1, "ALLOWLIST"); err != nil {
		t.Fatal(err)
	}
	if err := SubmitVote(2, 1, "ALLOWLIST"); err == nil {
		t.Fatal("expected error voting on finalized proposal")
	}
}
