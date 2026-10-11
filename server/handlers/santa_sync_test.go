package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"krampus/server/database"

	"github.com/gin-gonic/gin"
)

// EventUpload must store execution_time as a bare UTC string so it compares
// correctly against datetime('now', ...) and the bare-format timestamps every
// other column uses. Binding a raw time.Time would make the driver append a
// timezone-offset suffix that skews lexical datetime comparisons.
func TestEventUploadStoresUTCExecutionTime(t *testing.T) {
	initTestDB(t)
	exec(t, `INSERT INTO machines (machine_id) VALUES ('m1')`)

	const unixTime = 1767225600 // 2026-01-01 00:00:00 UTC
	body := `{"events":[{"file_sha256":"h1","file_path":"/bin/x",` +
		`"execution_time":1767225600,"decision":"BLOCK_BINARY"}]}`

	router := gin.New()
	router.POST("/eventupload/:machine_id", EventUpload)
	req := httptest.NewRequest(http.MethodPost, "/eventupload/m1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	// CAST avoids the driver's DATETIME decltype conversion so the assertion
	// sees the raw stored text.
	var stored string
	if err := database.DB.QueryRow(
		`SELECT CAST(execution_time AS TEXT) FROM events WHERE file_hash = 'h1'`,
	).Scan(&stored); err != nil {
		t.Fatalf("query stored execution_time: %v", err)
	}

	want := time.Unix(unixTime, 0).UTC().Format("2006-01-02 15:04:05")
	if stored != want {
		t.Fatalf("stored execution_time=%q, want bare UTC %q", stored, want)
	}
}
