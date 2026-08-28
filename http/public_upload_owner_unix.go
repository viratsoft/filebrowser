//go:build linux || darwin || freebsd || openbsd

package fbhttp

import (
	"errors"
	"os"
	"syscall"

	"github.com/spf13/afero"
)

// publicUploadOwner is deliberately internal. Its values come only from a
// Stat of the already-authorized shared directory, never from a share request.
type publicUploadOwner struct {
	uid int
	gid int
}

type publicUploadChowner interface {
	Chown(name string, uid, gid int) error
}

func canMatchPublicUploadOwner() bool {
	return os.Geteuid() == 0
}

func publicUploadOwnerFor(info os.FileInfo) (publicUploadOwner, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return publicUploadOwner{}, errors.New("could not determine shared folder owner")
	}
	return publicUploadOwner{uid: int(stat.Uid), gid: int(stat.Gid)}, nil
}

func applyPublicUploadOwner(fs afero.Fs, path string, owner publicUploadOwner) error {
	chowner, ok := fs.(publicUploadChowner)
	if !ok {
		return errors.New("filesystem does not support changing file ownership")
	}
	return chowner.Chown(path, owner.uid, owner.gid)
}
