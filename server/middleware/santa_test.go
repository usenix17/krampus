package middleware

import (
	"net/http/httptest"
	"testing"

	"krampus/server/config"

	"github.com/gin-gonic/gin"
)

// santaAuthStatus wires SantaAuthMiddleware ahead of a handler that records a
// hit, returning the HTTP status and whether the handler ran. It sets
// config.AppConfig for the duration of the call and restores it afterward.
func santaAuthStatus(t *testing.T, key, enforce string, provided string, viaBearer bool) (int, bool) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	prev := config.AppConfig
	config.AppConfig = &config.Config{
		SantaSyncKey:     key,
		SantaSyncEnforce: enforce == "true",
	}
	defer func() { config.AppConfig = prev }()

	reached := false
	r := gin.New()
	r.POST("/preflight/:machine_id", SantaAuthMiddleware(), func(c *gin.Context) {
		reached = true
		c.Status(200)
	})

	req := httptest.NewRequest("POST", "/preflight/some-machine", nil)
	if provided != "" {
		if viaBearer {
			req.Header.Set("Authorization", "Bearer "+provided)
		} else {
			req.Header.Set("X-Santa-Key", provided)
		}
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code, reached
}

func TestSantaAuthMiddlewareEnforceBadKeyRejected(t *testing.T) {
	code, reached := santaAuthStatus(t, "correct-key", "true", "wrong-key", false)
	if code != 401 || reached {
		t.Errorf("enforce=true bad key: code=%d reached=%v, want 401/false", code, reached)
	}
}

func TestSantaAuthMiddlewareEnforceMissingKeyRejected(t *testing.T) {
	code, reached := santaAuthStatus(t, "correct-key", "true", "", false)
	if code != 401 || reached {
		t.Errorf("enforce=true missing key: code=%d reached=%v, want 401/false", code, reached)
	}
}

func TestSantaAuthMiddlewareEnforceValidKeyAccepted(t *testing.T) {
	code, reached := santaAuthStatus(t, "correct-key", "true", "correct-key", false)
	if code != 200 || !reached {
		t.Errorf("enforce=true valid key: code=%d reached=%v, want 200/true", code, reached)
	}
}

func TestSantaAuthMiddlewareEnforceValidKeyViaBearerAccepted(t *testing.T) {
	code, reached := santaAuthStatus(t, "correct-key", "true", "correct-key", true)
	if code != 200 || !reached {
		t.Errorf("enforce=true valid bearer key: code=%d reached=%v, want 200/true", code, reached)
	}
}

func TestSantaAuthMiddlewareLogOnlyBadKeyAllowed(t *testing.T) {
	code, reached := santaAuthStatus(t, "correct-key", "false", "wrong-key", false)
	if code != 200 || !reached {
		t.Errorf("enforce=false bad key: code=%d reached=%v, want 200/true", code, reached)
	}
}

func TestSantaAuthMiddlewareLogOnlyMissingKeyAllowed(t *testing.T) {
	code, reached := santaAuthStatus(t, "correct-key", "false", "", false)
	if code != 200 || !reached {
		t.Errorf("enforce=false missing key: code=%d reached=%v, want 200/true", code, reached)
	}
}

func TestSantaAuthMiddlewareLogOnlyValidKeyAllowed(t *testing.T) {
	code, reached := santaAuthStatus(t, "correct-key", "false", "correct-key", false)
	if code != 200 || !reached {
		t.Errorf("enforce=false valid key: code=%d reached=%v, want 200/true", code, reached)
	}
}

func TestSantaAuthMiddlewareEmptyKeyOpenRegardlessOfEnforceTrue(t *testing.T) {
	code, reached := santaAuthStatus(t, "", "true", "", false)
	if code != 200 || !reached {
		t.Errorf("empty key enforce=true: code=%d reached=%v, want 200/true", code, reached)
	}
}

func TestSantaAuthMiddlewareEmptyKeyOpenRegardlessOfEnforceFalse(t *testing.T) {
	code, reached := santaAuthStatus(t, "", "false", "", false)
	if code != 200 || !reached {
		t.Errorf("empty key enforce=false: code=%d reached=%v, want 200/true", code, reached)
	}
}

// machineIDStatus wires ValidateMachineID ahead of a handler that records a
// hit, returning the HTTP status and whether the handler ran.
func machineIDStatus(t *testing.T, machineID string) (int, bool) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	reached := false
	r := gin.New()
	r.POST("/preflight/:machine_id", ValidateMachineID(), func(c *gin.Context) {
		reached = true
		c.Status(200)
	})
	req := httptest.NewRequest("POST", "/preflight/"+machineID, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code, reached
}

func TestValidateMachineIDValid(t *testing.T) {
	code, reached := machineIDStatus(t, "0A1B2C3D-4E5F-6071-8293-A4B5C6D7E8F9")
	if code != 200 || !reached {
		t.Errorf("valid machine id rejected: code=%d reached=%v", code, reached)
	}
}

func TestValidateMachineIDTraversal(t *testing.T) {
	// A raw "../etc" cannot survive routing: gin splits on the literal slash
	// (404 before middleware). "..%20etc" decodes to ".. etc", a single
	// segment whose space is outside the allowlist, so the validator aborts.
	code, reached := machineIDStatus(t, "..%20etc")
	if code != 400 || reached {
		t.Errorf("traversal/junk id accepted: code=%d reached=%v", code, reached)
	}
}

func TestValidateMachineIDTooLong(t *testing.T) {
	long := make([]byte, 300)
	for i := range long {
		long[i] = 'a'
	}
	code, reached := machineIDStatus(t, string(long))
	if code != 400 || reached {
		t.Errorf("300-char id accepted: code=%d reached=%v", code, reached)
	}
}

// Empty machine_id is unroutable: POST /preflight/ has no param segment, so
// gin's router returns 404 before any middleware runs. The valid and invalid
// cases above cover the validator's behavior.
func TestValidateMachineIDEmptyUnroutable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/preflight/:machine_id", ValidateMachineID(), func(c *gin.Context) {
		c.Status(200)
	})
	req := httptest.NewRequest("POST", "/preflight/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Errorf("empty machine id: code=%d, want 404 (unroutable)", w.Code)
	}
}
