package services

import (
	"encoding/json"
	"io"
	"krampus/server/models"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func strptr(s string) *string { return &s }

func TestBuildNewProposalPayload(t *testing.T) {
	created := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	p := models.Proposal{
		ID:             7,
		Identifier:     "ABCDE12345",
		RuleType:       "TEAMID",
		ProposedPolicy: "ALLOWLIST",
		CustomMessage:  strptr("Acme Corp app"),
		CreatedAt:      created,
	}

	payload := buildNewProposalPayload(p, "alice", strptr("alice@example.com"), "https://krampus.example.com/")

	if len(payload.Embeds) != 1 {
		t.Fatalf("expected 1 embed, got %d", len(payload.Embeds))
	}
	embed := payload.Embeds[0]

	if embed.Color != 0x2ecc71 {
		t.Errorf("allowlist proposal should be green, got %#x", embed.Color)
	}
	if embed.URL != "https://krampus.example.com/proposals" {
		t.Errorf("unexpected URL %q", embed.URL)
	}
	if embed.Timestamp != "2026-07-20T10:00:00Z" {
		t.Errorf("unexpected timestamp %q", embed.Timestamp)
	}

	joined := ""
	for _, f := range embed.Fields {
		joined += f.Name + "=" + f.Value + ";"
	}
	for _, want := range []string{"ABCDE12345", "TEAMID", "ALLOWLIST", "alice@example.com", "Acme Corp app"} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected fields to contain %q, got %q", want, joined)
		}
	}
}

func TestPostDiscordWebhook(t *testing.T) {
	var gotContentType string
	var gotBody discordWebhookPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	payload := discordWebhookPayload{Username: "Krampus", Embeds: []discordEmbed{{Title: "hi"}}}
	if err := postDiscordWebhook(srv.URL, payload); err != nil {
		t.Fatalf("postDiscordWebhook returned error: %v", err)
	}
	if gotContentType != "application/json" {
		t.Errorf("content type = %q, want application/json", gotContentType)
	}
	if gotBody.Username != "Krampus" || len(gotBody.Embeds) != 1 {
		t.Errorf("server received unexpected payload: %+v", gotBody)
	}
}

func TestPostDiscordWebhookNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	if err := postDiscordWebhook(srv.URL, discordWebhookPayload{}); err == nil {
		t.Error("expected error on non-2xx response, got nil")
	}
}

func TestBuildNewProposalPayloadBlocklistColor(t *testing.T) {
	p := models.Proposal{ProposedPolicy: "BLOCKLIST", CreatedAt: time.Unix(0, 0).UTC()}
	payload := buildNewProposalPayload(p, "bob", nil, "")
	if payload.Embeds[0].Color != 0xe74c3c {
		t.Errorf("blocklist proposal should be red, got %#x", payload.Embeds[0].Color)
	}
	if payload.Embeds[0].URL != "" {
		t.Errorf("expected no URL when baseURL empty, got %q", payload.Embeds[0].URL)
	}
}
