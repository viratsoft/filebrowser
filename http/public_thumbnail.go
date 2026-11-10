package fbhttp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"time"

	"github.com/filebrowser/filebrowser/v2/files"
	"github.com/filebrowser/filebrowser/v2/img"
)

// Bound the extra public read path independently of authenticated downloads.
const (
	maxPublicThumbnailSourceBytes int64 = 64 << 20
	maxPublicThumbnailPixels      int64 = 25_000_000
)

type publicThumbnailSourceReader struct {
	ctx    context.Context
	reader io.Reader
}

var publicImagePreviewSlots = make(chan struct{}, 2)

func (r publicThumbnailSourceReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

// Public thumbnails are an additional read path, so every request (including
// cache hits) must revalidate the share and its owner's current permissions.
func publicThumbnailHandler(imgSvc ImgService, cache FileCache) handleFunc {
	return publicImagePreviewHandler(imgSvc, cache, true)
}

func publicPreviewHandler(imgSvc ImgService, cache FileCache) handleFunc {
	return publicImagePreviewHandler(imgSvc, cache, false)
}

func publicImagePreviewHandler(imgSvc ImgService, cache FileCache, thumbnail bool) handleFunc {
	// Bound source hashing/open files as well as the image service's decoder
	// concurrency. Extra public requests wait without opening a source file.
	slots := publicImagePreviewSlots
	size, mode := 256, img.ResizeModeFill
	if !thumbnail {
		size, mode = 1080, img.ResizeModeFit
	}
	authorized := withHashFileOptions(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
		if d.rawShare.UploadOnly || (thumbnail && (d.user.HideShareThumbnails || !d.server.EnableThumbnails)) || (!thumbnail && !d.server.ResizePreview) {
			return http.StatusForbidden, nil
		}
		file := d.raw.(*files.FileInfo)
		if file.IsDir || !file.Mode.IsRegular() {
			return http.StatusNotFound, nil
		}
		if _, err := imgSvc.FormatFromExtension(file.Extension); err != nil {
			return http.StatusUnsupportedMediaType, nil
		}
		if file.Size > maxPublicThumbnailSourceBytes {
			return http.StatusRequestEntityTooLarge, nil
		}
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		case <-r.Context().Done():
			return errToStatus(r.Context().Err()), r.Context().Err()
		}
		reader, err := file.Fs.Open(file.Path)
		if err != nil {
			return errToStatus(err), err
		}
		defer reader.Close()
		// SMB tools and backup restores can replace an image while preserving
		// both its timestamp and byte length. Metadata alone could then expose
		// pixels from the previous file. Hash the authorized, bounded source;
		// cached JPEGs still avoid repeated image decoding and resizing.
		digest := sha256.New()
		n, err := io.Copy(digest, publicThumbnailSourceReader{ctx: r.Context(), reader: io.LimitReader(reader, maxPublicThumbnailSourceBytes+1)})
		if err != nil {
			return errToStatus(err), err
		}
		if n > maxPublicThumbnailSourceBytes {
			return http.StatusRequestEntityTooLarge, nil
		}
		key := fmt.Sprintf("public-image-v3-%d-%d-%x-%x", size, mode, file.RealPath(), digest.Sum(nil))
		thumbnail, cached, err := cache.Load(r.Context(), key)
		if err != nil {
			// Caching is optional. A read outage can still serve a freshly
			// generated preview through the same bounded image service.
			cached = false
		}
		if !cached {
			if _, err := reader.Seek(0, io.SeekStart); err != nil {
				return errToStatus(err), err
			}
			// A small compressed upload can still expand into a very large bitmap.
			// Bound public decode memory before invoking the full image decoder.
			config, _, err := image.DecodeConfig(publicThumbnailSourceReader{ctx: r.Context(), reader: io.LimitReader(reader, 1<<20)})
			if err != nil {
				if r.Context().Err() != nil {
					return errToStatus(r.Context().Err()), r.Context().Err()
				}
				return http.StatusUnsupportedMediaType, nil
			}
			if config.Width <= 0 || config.Height <= 0 || int64(config.Width) > maxPublicThumbnailPixels/int64(config.Height) {
				return http.StatusRequestEntityTooLarge, nil
			}
			if _, err := reader.Seek(0, io.SeekStart); err != nil {
				return errToStatus(err), err
			}
			// Hash the exact stream decoded on a miss, too. If an external writer
			// changes the open file between the two reads, never cache the new
			// pixels under the earlier source's fingerprint. Stream rather than
			// retaining entire originals in memory for waiting requests.
			decodedDigest := sha256.New()
			limited := &io.LimitedReader{R: publicThumbnailSourceReader{ctx: r.Context(), reader: reader}, N: maxPublicThumbnailSourceBytes + 1}
			source := io.TeeReader(limited, decodedDigest)
			var output bytes.Buffer
			// Medium quality avoids trusting an embedded EXIF thumbnail. Always
			// re-encode as a small JPEG; never fall back to serving the raw file.
			err = imgSvc.Resize(r.Context(), source, size, size, &output,
				img.WithMode(mode), img.WithQuality(img.QualityMedium), img.WithFormat(img.FormatJpeg))
			if err != nil {
				if errors.Is(err, img.ErrUnsupportedFormat) {
					return http.StatusUnsupportedMediaType, nil
				}
				if errors.Is(err, img.ErrImageTooLarge) {
					return http.StatusRequestEntityTooLarge, nil
				}
				return errToStatus(err), err
			}
			if _, err := io.Copy(io.Discard, source); err != nil {
				return errToStatus(err), err
			}
			if limited.N == 0 {
				return http.StatusRequestEntityTooLarge, nil
			}
			key = fmt.Sprintf("public-image-v3-%d-%d-%x-%x", size, mode, file.RealPath(), decodedDigest.Sum(nil))
			thumbnail = output.Bytes()
			// A cache outage must not turn an otherwise valid thumbnail into a
			// broken image. Store synchronously; do not spawn unbounded goroutines.
			_ = cache.Store(r.Context(), key, thumbnail)
		}
		w.Header().Set("Content-Type", "image/jpeg")
		// A restored mtime is not a valid HTTP validator for these pixels.
		// no-store responses deliberately omit Last-Modified as well.
		http.ServeContent(w, r, file.Name, time.Time{}, bytes.NewReader(thumbnail))
		return 0, nil
	}, false)
	return func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
		// Do not let browser/proxy caches bypass a later disable, revocation,
		// password change, or expiry. The internal cache still saves resize work.
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		status, err := authorized(w, r.WithContext(ctx), d)
		if errors.Is(err, context.DeadlineExceeded) {
			return http.StatusGatewayTimeout, nil
		}
		return status, err
	}
}
