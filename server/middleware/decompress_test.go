package middleware

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func runDecompress(t *testing.T, body []byte, contentType, encoding string) (*httptest.ResponseRecorder, []byte) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	var got []byte
	r.POST("/x", Decompress(), func(c *gin.Context) {
		b, _ := io.ReadAll(c.Request.Body)
		got = b
		c.Status(http.StatusOK)
	})
	req := httptest.NewRequest("POST", "/x", bytes.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if encoding != "" {
		req.Header.Set("Content-Encoding", encoding)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w, got
}

func TestEmptyJSONBodyDoesNotPanic(t *testing.T) {
	w, got := runDecompress(t, nil, "application/json", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	if len(got) != 0 {
		t.Fatalf("body=%q, want empty", got)
	}
}

func TestPlainJSONPassesThrough(t *testing.T) {
	_, got := runDecompress(t, []byte(`{"a":1}`), "application/json", "")
	if string(got) != `{"a":1}` {
		t.Fatalf("got %q", got)
	}
}

func TestGzipEncoding(t *testing.T) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write([]byte(`{"a":1}`))
	zw.Close()
	_, got := runDecompress(t, buf.Bytes(), "application/json", "gzip")
	if string(got) != `{"a":1}` {
		t.Fatalf("got %q", got)
	}
}

func TestZlibDeflateEncoding(t *testing.T) {
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	zw.Write([]byte(`{"a":1}`))
	zw.Close()
	_, got := runDecompress(t, buf.Bytes(), "application/json", "deflate")
	if string(got) != `{"a":1}` {
		t.Fatalf("got %q", got)
	}
}

func TestSniffedZlibWithoutEncodingHeader(t *testing.T) {
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	zw.Write([]byte(`{"a":1}`))
	zw.Close()
	_, got := runDecompress(t, buf.Bytes(), "application/json", "")
	if string(got) != `{"a":1}` {
		t.Fatalf("got %q", got)
	}
}

func TestCorruptGzipRejected(t *testing.T) {
	w, _ := runDecompress(t, []byte("\x1f\x8bnot-gzip"), "application/json", "gzip")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}

func TestNonJSONWithoutEncodingUntouched(t *testing.T) {
	_, got := runDecompress(t, []byte("rawbytes"), "application/x-protobuf", "")
	if string(got) != "rawbytes" {
		t.Fatalf("got %q", got)
	}
}

func TestZlibBombRejected(t *testing.T) {
	// 300 MiB of zeros compresses to well under 10 MiB but inflates past
	// the 64 MiB decompressed cap, so the request must be rejected.
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	zeros := make([]byte, 1<<20)
	for i := 0; i < 300; i++ {
		zw.Write(zeros)
	}
	zw.Close()
	if buf.Len() > 10<<20 {
		t.Fatalf("bomb payload too large: %d bytes", buf.Len())
	}
	w, got := runDecompress(t, buf.Bytes(), "application/json", "deflate")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
	if len(got) != 0 {
		t.Fatalf("handler ran with %d bytes, want aborted request", len(got))
	}
}

func TestRawDeflateEncoding(t *testing.T) {
	var buf bytes.Buffer
	fw, err := flate.NewWriter(&buf, flate.DefaultCompression)
	if err != nil {
		t.Fatal(err)
	}
	fw.Write([]byte(`{"a":1}`))
	fw.Close()
	_, got := runDecompress(t, buf.Bytes(), "application/json", "deflate")
	if string(got) != `{"a":1}` {
		t.Fatalf("got %q", got)
	}
}

func TestContentEncodingHeaderDeleted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	var seen string
	r.POST("/x", Decompress(), func(c *gin.Context) {
		seen = c.GetHeader("Content-Encoding")
		c.Status(http.StatusOK)
	})
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write([]byte(`{"a":1}`))
	zw.Close()
	req := httptest.NewRequest("POST", "/x", bytes.NewReader(buf.Bytes()))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	if seen != "" {
		t.Fatalf("Content-Encoding=%q after inflate, want deleted", seen)
	}
}
