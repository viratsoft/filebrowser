//go:build linux || darwin || freebsd || openbsd

package fbhttp

import (
	"os"
	"testing"
)

func TestPublicUploadOwnerComesFromFilesystemMetadata(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "owner")
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := publicUploadOwnerFor(info); err != nil {
		t.Fatalf("expected owner to be read from filesystem metadata: %v", err)
	}
}
