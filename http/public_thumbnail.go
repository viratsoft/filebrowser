package fbhttp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"time"

	"github.com/filebrowser/filebrowser/v2/files"
	"github.com/filebrowser/filebrowser/v2/img"
	"github.com/jellydator/ttlcache/v3"
	"github.com/marusama/semaphore/v2"
)

// Bound the extra public read path independently of authenticated downloads.
const (
	maxPublicThumbnailSourceBytes      int64 = 64 << 20
	maxPublicThumbnailSnapshotBytes    int64 = 16 << 20
	publicThumbnailMemoryTTL                 = 5 * time.Minute
	publicThumbnailMemoryEntries             = 128
	maxPublicThumbnailMemoryEntryBytes       = 96 << 10
	publicThumbnailPixelBudget               = img.MaxImageWidth * img.MaxImageHeight
)

type publicThumbnailSourceReader struct {
	ctx    context.Context
	reader io.Reader
}

var publicImagePreviewSlots = make(chan struct{}, 8)
var publicImageFingerprintSlots = make(chan struct{}, 4)
var publicThumbnailSnapshotSlots = make(chan struct{}, 4)
var publicImageResizeSlots = make(chan struct{}, 2)
var publicImageDecodeBudget = semaphore.New(publicThumbnailPixelBudget)

func (r publicThumbnailSourceReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

// fingerprintPublicThumbnail retains a bounded snapshot when possible so a
// cache miss can decode the exact bytes already read for the fingerprint.
func fingerprintPublicThumbnail(ctx context.Context, reader io.Reader, snapshotSize int64) ([]byte, []byte, error) {
	source := &io.LimitedReader{
		R: publicThumbnailSourceReader{ctx: ctx, reader: reader},
		N: maxPublicThumbnailSourceBytes + 1,
	}
	digest := sha256.New()
	if snapshotSize > 0 && snapshotSize <= maxPublicThumbnailSnapshotBytes {
		snapshot := make([]byte, snapshotSize+1)
		n, err := io.ReadFull(source, snapshot)
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			sum := sha256.Sum256(snapshot[:n])
			return sum[:], snapshot[:n], nil
		}
		if err != nil {
			return nil, nil, err
		}
		// The file grew since stat. Complete the digest without retaining it.
		_, _ = digest.Write(snapshot)
	}
	if _, err := io.Copy(digest, source); err != nil {
		return nil, nil, err
	}
	if source.N == 0 {
		return nil, nil, img.ErrImageTooLarge
	}
	return digest.Sum(nil), nil, nil
}

// Both authenticated and public callers must authorize the file before rendering.
type thumbnailRenderer func(context.Context, *files.FileInfo) ([]byte, error)

func newImageRenderer(imgSvc ImgService, cache FileCache, previewSize PreviewSize) thumbnailRenderer {
	isThumbnail := previewSize == PreviewSizeThumb
	var memory *ttlcache.Cache[string, []byte]
	if isThumbnail {
		memory = ttlcache.New(
			ttlcache.WithCapacity[string, []byte](publicThumbnailMemoryEntries),
			ttlcache.WithDisableTouchOnHit[string, []byte](),
		)
	}
	size, resizeOptions := previewResizeSpec(previewSize)
	resizeOptions = append(resizeOptions, img.WithFormat(img.FormatJpeg),
		img.WithoutEmbeddedThumbnail(), img.WithDecodeBudget(publicImageDecodeBudget, publicThumbnailPixelBudget))
	return func(ctx context.Context, file *files.FileInfo) ([]byte, error) {
		if file.Size > maxPublicThumbnailSourceBytes {
			return nil, img.ErrImageTooLarge
		}
		// Cache hits can check their source while other requests decode images.
		select {
		case publicImagePreviewSlots <- struct{}{}:
			defer func() { <-publicImagePreviewSlots }()
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		reader, err := file.Fs.Open(file.Path)
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		// SMB tools and backup restores can replace an image while preserving
		// both its timestamp and byte length. Metadata alone could then expose
		// pixels from the previous file. Hash the authorized, bounded source;
		// cached JPEGs still avoid repeated image decoding and resizing.
		var snapshotSize int64
		if isThumbnail && file.Size > 0 && file.Size <= maxPublicThumbnailSnapshotBytes {
			select {
			case publicThumbnailSnapshotSlots <- struct{}{}:
				defer func() { <-publicThumbnailSnapshotSlots }()
				snapshotSize = file.Size
			default:
				// Hash as a stream when the snapshot budget is occupied.
			}
		}
		select {
		case publicImageFingerprintSlots <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		fingerprint, snapshot, err := fingerprintPublicThumbnail(ctx, reader, snapshotSize)
		<-publicImageFingerprintSlots
		if err != nil {
			return nil, err
		}
		realPath := file.RealPath()
		key := fmt.Sprintf("public-image-v4-%d-%x-%x", previewSize, realPath, fingerprint)
		var thumbnail []byte
		if memory != nil {
			if entry := memory.Get(key); entry != nil {
				thumbnail = entry.Value()
			}
		}
		if len(thumbnail) == 0 {
			data, cached, cacheErr := cache.Load(ctx, key)
			if cacheErr == nil && cached {
				thumbnail = data
			}
		}
		if len(thumbnail) == 0 {
			select {
			case publicImageResizeSlots <- struct{}{}:
				defer func() { <-publicImageResizeSlots }()
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			var source io.Reader
			var decodedDigest hash.Hash
			var limited *io.LimitedReader
			if snapshot != nil {
				source = publicThumbnailSourceReader{ctx: ctx, reader: bytes.NewReader(snapshot)}
			} else {
				if _, err := reader.Seek(0, io.SeekStart); err != nil {
					return nil, err
				}
				// Fingerprint the actual decode stream when a snapshot was unavailable.
				decodedDigest = sha256.New()
				limited = &io.LimitedReader{R: publicThumbnailSourceReader{ctx: ctx, reader: reader}, N: maxPublicThumbnailSourceBytes + 1}
				source = io.TeeReader(limited, decodedDigest)
			}
			var output bytes.Buffer
			// Only serve a JPEG derived from the authorized main image pixels.
			err = imgSvc.Resize(ctx, source, size, size, &output, resizeOptions...)
			if err != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				return nil, err
			}
			if limited != nil {
				if _, err := io.Copy(io.Discard, source); err != nil {
					return nil, err
				}
				if limited.N == 0 {
					return nil, img.ErrImageTooLarge
				}
				key = fmt.Sprintf("public-image-v4-%d-%x-%x", previewSize, realPath, decodedDigest.Sum(nil))
			}
			thumbnail = output.Bytes()
			// A cache outage must not turn an otherwise valid thumbnail into a
			// broken image. Store synchronously; do not spawn unbounded goroutines.
			_ = cache.Store(ctx, key, thumbnail)
		}
		if memory != nil && len(thumbnail) <= maxPublicThumbnailMemoryEntryBytes && memory.Get(key) == nil {
			memory.Set(key, bytes.Clone(thumbnail), publicThumbnailMemoryTTL)
		}
		return thumbnail, nil
	}
}

func publicThumbnailHandler(imgSvc ImgService, render thumbnailRenderer) handleFunc {
	return publicImagePreviewHandler(imgSvc, render, true)
}

func publicPreviewHandler(imgSvc ImgService, cache FileCache) handleFunc {
	return publicImagePreviewHandler(imgSvc, newImageRenderer(imgSvc, cache, PreviewSizeBig), false)
}

// Every public request, including cache hits, revalidates the share and file.
func publicImagePreviewHandler(imgSvc ImgService, render thumbnailRenderer, thumbnail bool) handleFunc {
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
		pixels, err := render(r.Context(), file)
		switch {
		case errors.Is(err, img.ErrUnsupportedFormat):
			return http.StatusUnsupportedMediaType, nil
		case errors.Is(err, img.ErrImageTooLarge):
			return http.StatusRequestEntityTooLarge, nil
		case err != nil:
			return errToStatus(err), err
		}
		w.Header().Set("Content-Type", "image/jpeg")
		http.ServeContent(w, r, file.Name, time.Time{}, bytes.NewReader(pixels))
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
