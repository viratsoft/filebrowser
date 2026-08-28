//go:build !linux && !darwin && !freebsd && !openbsd

package fbhttp

import (
	"errors"
	"os"

	"github.com/spf13/afero"
)

type publicUploadOwner struct{}

func canMatchPublicUploadOwner() bool { return false }

func publicUploadOwnerFor(os.FileInfo) (publicUploadOwner, error) {
	return publicUploadOwner{}, errors.New("matching uploaded file ownership is not supported on this platform")
}

func applyPublicUploadOwner(afero.Fs, string, publicUploadOwner) error {
	return errors.New("matching uploaded file ownership is not supported on this platform")
}
