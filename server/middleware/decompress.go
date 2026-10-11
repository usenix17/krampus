package middleware

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/gin-gonic/gin"
)

const maxDecompressed = 64 << 20 // decompressed cap; compressed is capped at 10 MiB upstream

// Decompress transparently inflates gzip/deflate request bodies. Santa sends
// zlib-wrapped deflate, sometimes without a Content-Encoding header, so JSON
// requests with no encoding header are sniffed.
func Decompress() gin.HandlerFunc {
	return func(c *gin.Context) {
		encoding := strings.ToLower(c.GetHeader("Content-Encoding"))
		isJSON := strings.Contains(c.GetHeader("Content-Type"), "json")

		if encoding == "" && !isJSON {
			c.Next()
			return
		}

		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.AbortWithStatusJSON(400, gin.H{"error": "failed to read body"})
			return
		}
		c.Request.Body.Close()

		out, err := inflate(body, encoding)
		if err != nil {
			log.Printf("Decompress: inflate failed (encoding=%q, %d bytes): %v", encoding, len(body), err)
			c.AbortWithStatusJSON(400, gin.H{"error": "failed to decompress body"})
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(out))
		c.Request.Header.Del("Content-Encoding")
		c.Next()
	}
}

// inflate decodes body per the declared encoding; with no encoding it sniffs
// compressed JSON. Unknown encodings and already-plain bodies pass through.
func inflate(body []byte, encoding string) ([]byte, error) {
	if len(body) == 0 {
		return body, nil
	}
	switch encoding {
	case "gzip":
		return gunzip(body)
	case "deflate":
		if out, err := unzlib(body); err == nil {
			return out, nil
		}
		// Raw DEFLATE has no magic bytes, so corrupt input may "decode" to
		// garbage here; that is intentionally left to fail at JSON parse
		// downstream.
		return deflateRaw(body)
	case "":
		if body[0] == '{' || body[0] == '[' {
			return body, nil
		}
		if out, err := gunzip(body); err == nil {
			return out, nil
		}
		if out, err := unzlib(body); err == nil {
			return out, nil
		}
		if out, err := deflateRaw(body); err == nil && len(out) > 0 {
			return out, nil
		}
		return body, nil
	default:
		return body, nil
	}
}

// readCapped reads at most maxDecompressed bytes, erroring past the cap so a
// small compressed bomb cannot balloon into an unbounded allocation.
func readCapped(r io.Reader) ([]byte, error) {
	out, err := io.ReadAll(io.LimitReader(r, maxDecompressed+1))
	if err != nil {
		return nil, err
	}
	if len(out) > maxDecompressed {
		return nil, fmt.Errorf("decompressed body exceeds %d bytes", maxDecompressed)
	}
	return out, nil
}

func gunzip(body []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	out, err := readCapped(r)
	if err != nil {
		return nil, fmt.Errorf("gzip: %w", err)
	}
	return out, nil
}

func unzlib(body []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	out, err := readCapped(r)
	if err != nil {
		return nil, fmt.Errorf("zlib: %w", err)
	}
	return out, nil
}

func deflateRaw(body []byte) ([]byte, error) {
	r := flate.NewReader(bytes.NewReader(body))
	defer r.Close()
	out, err := readCapped(r)
	if err != nil {
		return nil, fmt.Errorf("deflate: %w", err)
	}
	return out, nil
}
