package fbhttp

import (
	"crypto/tls"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"
)

func TestUploadSessionSignatureBindsFolderToShare(t *testing.T) {
	key := []byte("test-signing-key")
	session := uploadSession{
		ID:      "visitor-session-id",
		Expires: time.Now().Add(time.Hour).Unix(),
		Folder:  "upload_2026-08-29_09-42-18_PM_random",
	}
	cookie, err := signUploadSession(session, "share-a", key)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := verifyUploadSession(cookie, "share-a", key); !ok || got.Folder != session.Folder {
		t.Fatalf("expected valid signed session, got %#v ok=%v", got, ok)
	}
	if _, ok := verifyUploadSession(cookie, "share-b", key); ok {
		t.Fatal("session cookie must not be valid for another share")
	}

	parts := strings.Split(cookie, ".")
	if len(parts) != 2 {
		t.Fatalf("unexpected cookie format %q", cookie)
	}
	replacement := byte('A')
	if parts[0][0] == replacement {
		replacement = 'B'
	}
	parts[0] = string(replacement) + parts[0][1:]
	if _, ok := verifyUploadSession(strings.Join(parts, "."), "share-a", key); ok {
		t.Fatal("tampered session payload must be rejected")
	}
}

func TestExpiredUploadSessionIsRejected(t *testing.T) {
	key := []byte("test-signing-key")
	cookie, err := signUploadSession(uploadSession{ID: "expired", Expires: time.Now().Add(-time.Second).Unix()}, "share-a", key)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := verifyUploadSession(cookie, "share-a", key); ok {
		t.Fatal("expired session must be rejected")
	}
}

func TestTrackedUploadSessionsPruneExpiredEntries(t *testing.T) {
	store := uploadSessionStore{sessions: map[string]*uploadSessionFiles{
		"share:expired": {expires: time.Now().Add(-time.Second), files: map[string]struct{}{}},
	}}
	session := uploadSession{ID: "active", Expires: time.Now().Add(time.Hour).Unix()}
	if err := store.ensureFiles("share", session); err != nil {
		t.Fatal(err)
	}
	if len(store.sessions) != 1 || store.sessions["share:active"] == nil {
		t.Fatalf("expected only the active session to remain, got %#v", store.sessions)
	}
}

func TestCreateVisitorSessionFolderIsIdempotent(t *testing.T) {
	fs := afero.NewMemMapFs()
	created, err := createVisitorSessionFolder(fs, "upload_2026-08-29_09-42-18_PM_random", 0o755)
	if err != nil || !created {
		t.Fatalf("expected a new visitor folder, created=%v err=%v", created, err)
	}
	created, err = createVisitorSessionFolder(fs, "upload_2026-08-29_09-42-18_PM_random", 0o755)
	if err != nil || created {
		t.Fatalf("expected existing visitor folder to be reused, created=%v err=%v", created, err)
	}
}

func TestRequestUsesHTTPS(t *testing.T) {
	plain := httptest.NewRequest("GET", "http://example.test", nil)
	if requestUsesHTTPS(plain) {
		t.Fatal("plain request must not set a Secure cookie")
	}
	tlsRequest := httptest.NewRequest("GET", "https://example.test", nil)
	tlsRequest.TLS = &tls.ConnectionState{}
	if !requestUsesHTTPS(tlsRequest) {
		t.Fatal("TLS request must set a Secure cookie")
	}
	proxied := httptest.NewRequest("GET", "http://example.test", nil)
	proxied.Header.Set("X-Forwarded-Proto", "https, http")
	if !requestUsesHTTPS(proxied) {
		t.Fatal("HTTPS proxy request must set a Secure cookie")
	}
}
