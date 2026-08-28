package fbhttp

import (
	"strings"
	"testing"
	"time"
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
