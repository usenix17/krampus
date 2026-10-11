package services

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

// assertWellFormedXML walks every token in the document and fails the test
// on the first parse error, proving the generated profile is valid XML.
func assertWellFormedXML(t *testing.T, doc string) {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader([]byte(doc)))
	// Apple's plist DTD isn't fetchable in tests; skip external entity/DTD
	// resolution errors are avoided by not validating against the DTD at all,
	// we only care that the token stream is well-formed.
	for {
		_, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("generated mobileconfig is not well-formed XML: %v", err)
		}
	}
}

func TestGenerateMobileConfigWithSyncKey(t *testing.T) {
	const key = "s3cr3t-sync-key"
	doc := GenerateMobileConfig("MACHINE-1", "LOCKDOWN", "https://sync.example.com", "Acme Corp", "admin", 600, key)

	if !strings.Contains(doc, "<key>SyncExtraHeaders</key>") {
		t.Error("expected SyncExtraHeaders key in mobileconfig when sync key is set")
	}
	if !strings.Contains(doc, "<key>X-Santa-Key</key>") {
		t.Error("expected X-Santa-Key header name in mobileconfig when sync key is set")
	}
	if !strings.Contains(doc, "<string>"+key+"</string>") {
		t.Errorf("expected sync key value %q to appear in mobileconfig", key)
	}

	assertWellFormedXML(t, doc)
}

func TestGenerateMobileConfigWithoutSyncKey(t *testing.T) {
	doc := GenerateMobileConfig("MACHINE-1", "LOCKDOWN", "https://sync.example.com", "Acme Corp", "admin", 600, "")

	if strings.Contains(doc, "SyncExtraHeaders") {
		t.Error("did not expect SyncExtraHeaders in mobileconfig when no sync key is set")
	}

	assertWellFormedXML(t, doc)
}

func TestGenerateMobileConfigEscapesSyncKey(t *testing.T) {
	const rawKey = `a&b<c>d`
	const escapedKey = "a&amp;b&lt;c&gt;d"
	doc := GenerateMobileConfig("MACHINE-1", "LOCKDOWN", "https://sync.example.com", "Acme Corp", "admin", 600, rawKey)

	if !strings.Contains(doc, "<string>"+escapedKey+"</string>") {
		t.Errorf("expected XML-escaped sync key %q in mobileconfig", escapedKey)
	}
	if strings.Contains(doc, "<string>"+rawKey+"</string>") {
		t.Error("raw unescaped sync key leaked into mobileconfig")
	}

	assertWellFormedXML(t, doc)
}
