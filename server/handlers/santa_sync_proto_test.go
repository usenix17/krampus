package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"krampus/server/database"

	"github.com/gin-gonic/gin"
	syncv1 "github.com/northpolesec/protos/sync"
	"google.golang.org/protobuf/proto"
)

// EventUpload must decode protobuf event uploads (Santa's default transport)
// and store them through the same path as JSON events. A protobuf body was
// previously dropped with only a log line.
func TestEventUploadStoresProtobufEvents(t *testing.T) {
	initTestDB(t)
	exec(t, `INSERT INTO machines (machine_id) VALUES ('m1')`)

	const unixTime = 1767225600 // 2026-01-01 00:00:00 UTC
	reqMsg := &syncv1.EventUploadRequest{
		MachineId: "m1",
		Events: []*syncv1.Event{{
			FileSha256:     "h1",
			FilePath:       "/bin/x",
			ExecutingUser:  "alice",
			ExecutionTime:  float64(unixTime),
			Decision:       syncv1.Decision_BLOCK_BINARY,
			SigningId:      "sign1",
			TeamId:         "team1",
			FileBundleId:   "com.example.app",
			FileBundleName: "Example",
			SigningChain: []*syncv1.Certificate{{
				Sha256: "cert1",
				Cn:     "Developer ID: Example",
			}},
		}},
	}
	raw, err := proto.Marshal(reqMsg)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	router := gin.New()
	router.POST("/eventupload/:machine_id", EventUpload)
	req := httptest.NewRequest(http.MethodPost, "/eventupload/m1", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/x-protobuf")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	var (
		filePath, decision, user, certSHA, certCN string
		signingID, teamID, bundleID, bundleName   string
		execTime                                  string
	)
	err = database.DB.QueryRow(
		`SELECT file_path, decision, executing_user, cert_sha256, cert_cn,
		        signing_id, team_id, bundle_id, bundle_name,
		        CAST(execution_time AS TEXT)
		 FROM events WHERE file_hash = 'h1'`,
	).Scan(&filePath, &decision, &user, &certSHA, &certCN,
		&signingID, &teamID, &bundleID, &bundleName, &execTime)
	if err != nil {
		t.Fatalf("protobuf event not stored: %v", err)
	}

	checks := []struct{ got, want, name string }{
		{filePath, "/bin/x", "file_path"},
		{decision, "BLOCK_BINARY", "decision"},
		{user, "alice", "executing_user"},
		{certSHA, "cert1", "cert_sha256"},
		{certCN, "Developer ID: Example", "cert_cn"},
		{signingID, "sign1", "signing_id"},
		{teamID, "team1", "team_id"},
		{bundleID, "com.example.app", "bundle_id"},
		{bundleName, "Example", "bundle_name"},
		{execTime, time.Unix(unixTime, 0).UTC().Format("2006-01-02 15:04:05"), "execution_time"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s=%q, want %q", c.name, c.got, c.want)
		}
	}

	// The response must itself be protobuf so Santa can decode it.
	if ct := w.Header().Get("Content-Type"); ct != "application/x-protobuf" {
		t.Errorf("response Content-Type=%q, want application/x-protobuf", ct)
	}
	var respMsg syncv1.EventUploadResponse
	if err := proto.Unmarshal(w.Body.Bytes(), &respMsg); err != nil {
		t.Errorf("response is not a valid EventUploadResponse: %v", err)
	}
}
