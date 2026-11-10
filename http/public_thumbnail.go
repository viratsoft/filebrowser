package fbhttp

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"

	"github.com/filebrowser/filebrowser/v2/files"
	"github.com/filebrowser/filebrowser/v2/img"
)

// Public thumbnails are an additional read path, so every request (including
// cache hits) must revalidate the share and its owner's current permissions.
func publicThumbnailHandler(imgSvc ImgService, cache FileCache) handleFunc {
	authorized := withHashFileOptions(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
		if d.rawShare.UploadOnly || d.user.HideShareThumbnails || !d.server.EnableThumbnails {
			return http.StatusForbidden, nil
		}
		file := d.raw.(*files.FileInfo)
		if file.IsDir || !file.Mode.IsRegular() {
			return http.StatusNotFound, nil
		}
		if _, err := imgSvc.FormatFromExtension(file.Extension); err != nil {
			return http.StatusUnsupportedMediaType, nil
		}
		// Separate from authenticated previews; include nanoseconds and size so
		// fast replacement of a file does not reuse an older public thumbnail.
		key := fmt.Sprintf("public-thumb-%x-%x-%x", file.RealPath(), file.ModTime.UnixNano(), file.Size)
		thumbnail, cached, err := cache.Load(r.Context(), key)
		if err != nil {
			// Caching is optional. A read outage can still serve a freshly
			// generated preview through the same bounded image service.
			cached = false
		}
		if !cached {
			reader, err := file.Fs.Open(file.Path)
			if err != nil {
				return errToStatus(err), err
			}
			var output bytes.Buffer
			// Medium quality avoids trusting an embedded EXIF thumbnail. Always
			// re-encode as a small JPEG; never fall back to serving the raw file.
			err = imgSvc.Resize(r.Context(), reader, 256, 256, &output,
				img.WithMode(img.ResizeModeFill), img.WithQuality(img.QualityMedium), img.WithFormat(img.FormatJpeg))
			closeErr := reader.Close()
			if err != nil {
				if errors.Is(err, img.ErrUnsupportedFormat) {
					return http.StatusUnsupportedMediaType, nil
				}
				return errToStatus(err), err
			}
			if closeErr != nil {
				return errToStatus(closeErr), closeErr
			}
			thumbnail = output.Bytes()
			// A cache outage must not turn an otherwise valid thumbnail into a
			// broken image. Store synchronously; do not spawn unbounded goroutines.
			_ = cache.Store(r.Context(), key, thumbnail)
		}
		w.Header().Set("Content-Type", "image/jpeg")
		http.ServeContent(w, r, file.Name, file.ModTime, bytes.NewReader(thumbnail))
		return 0, nil
	}, false)
	return func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
		// Do not let browser/proxy caches bypass a later disable, revocation,
		// password change, or expiry. The internal cache still saves resize work.
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		return authorized(w, r, d)
	}
}
