//go:generate go-enum --sql --marshal --file $GOFILE
package img

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"

	"github.com/disintegration/imaging"
	"github.com/dsoprea/go-exif/v3"
	"github.com/marusama/semaphore/v2"

	exifcommon "github.com/dsoprea/go-exif/v3/common"
)

// ErrUnsupportedFormat means the given image format is not supported.
var ErrUnsupportedFormat = errors.New("unsupported image format")

// ErrImageTooLarge means the image is too large to create a thumbnail.
var ErrImageTooLarge = errors.New("image too large for thumbnail generation")

// Maximum dimensions for thumbnail generation to prevent server crashes
const (
	MaxImageWidth  = 10000
	MaxImageHeight = 10000
)

// Service
type Service struct {
	sem semaphore.Semaphore
}

func New(workers int) *Service {
	return &Service{
		sem: semaphore.New(workers),
	}
}

// Format is an image file format.
/*
ENUM(
jpeg
png
gif
tiff
bmp
)
*/
type Format int

func (x Format) toImaging() imaging.Format {
	switch x {
	case FormatJpeg:
		return imaging.JPEG
	case FormatPng:
		return imaging.PNG
	case FormatGif:
		return imaging.GIF
	case FormatTiff:
		return imaging.TIFF
	case FormatBmp:
		return imaging.BMP
	default:
		return imaging.JPEG
	}
}

/*
ENUM(
high
medium
low
)
*/
type Quality int

func (x Quality) resampleFilter() imaging.ResampleFilter {
	switch x {
	case QualityHigh:
		return imaging.Lanczos
	case QualityMedium:
		return imaging.Box
	case QualityLow:
		return imaging.NearestNeighbor
	default:
		return imaging.Box
	}
}

/*
ENUM(
fit
fill
)
*/
type ResizeMode int

func (s *Service) FormatFromExtension(ext string) (Format, error) {
	format, err := imaging.FormatFromExtension(ext)
	if err != nil {
		return -1, ErrUnsupportedFormat
	}
	switch format {
	case imaging.JPEG:
		return FormatJpeg, nil
	case imaging.PNG:
		return FormatPng, nil
	case imaging.GIF:
		return FormatGif, nil
	case imaging.TIFF:
		return FormatTiff, nil
	case imaging.BMP:
		return FormatBmp, nil
	}
	return -1, ErrUnsupportedFormat
}

type resizeConfig struct {
	format                *Format
	resizeMode            ResizeMode
	quality               Quality
	decodeBudget          semaphore.Semaphore
	decodeBudgetCapacity  int
	skipEmbeddedThumbnail bool
}

type Option func(*resizeConfig)

func WithFormat(format Format) Option {
	return func(config *resizeConfig) {
		config.format = &format
	}
}

func WithMode(mode ResizeMode) Option {
	return func(config *resizeConfig) {
		config.resizeMode = mode
	}
}

func WithQuality(quality Quality) Option {
	return func(config *resizeConfig) {
		config.quality = quality
	}
}

// WithoutEmbeddedThumbnail derives thumbnails from the main image pixels.
func WithoutEmbeddedThumbnail() Option {
	return func(config *resizeConfig) { config.skipEmbeddedThumbnail = true }
}

// WithDecodeBudget bounds concurrent full-image decodes with a shared pixel budget.
// The dimensions come from the same header replayed to the full decoder.
func WithDecodeBudget(budget semaphore.Semaphore, capacity int) Option {
	return func(config *resizeConfig) {
		config.decodeBudget = budget
		config.decodeBudgetCapacity = capacity
	}
}

func (s *Service) Resize(ctx context.Context, in io.Reader, width, height int, out io.Writer, options ...Option) error {
	if err := s.sem.Acquire(ctx, 1); err != nil {
		return err
	}
	defer s.sem.Release(1)

	config := resizeConfig{
		resizeMode: ResizeModeFit,
		quality:    QualityMedium,
	}
	for _, option := range options {
		option(&config)
	}

	format, wrappedReader, pixels, err := s.detectFormat(in, config.decodeBudget != nil)
	if err != nil {
		return err
	}
	if config.format == nil {
		config.format = &format
	}

	if config.quality == QualityLow && format == FormatJpeg && !config.skipEmbeddedThumbnail {
		thm, newWrappedReader, errThm := getEmbeddedThumbnail(wrappedReader)
		wrappedReader = newWrappedReader
		if errThm == nil {
			_, err = out.Write(thm)
			if err == nil {
				return nil
			}
		}
	}
	if config.decodeBudget != nil {
		// Larger images take the whole budget and decode one at a time.
		weight := min(pixels*2, config.decodeBudgetCapacity)
		if err := config.decodeBudget.Acquire(ctx, weight); err != nil {
			return err
		}
		defer config.decodeBudget.Release(weight)
	}

	img, err := imaging.Decode(wrappedReader, imaging.AutoOrientation(true))
	if err != nil {
		return err
	}

	switch config.resizeMode {
	case ResizeModeFill:
		img = imaging.Fill(img, width, height, imaging.Center, config.quality.resampleFilter())
	case ResizeModeFit:
		fallthrough
	default:
		img = imaging.Fit(img, width, height, config.quality.resampleFilter())
	}

	return imaging.Encode(out, img, config.format.toImaging())
}

func (s *Service) detectFormat(in io.Reader, limitHeader bool) (Format, io.Reader, int, error) {
	buf := &bytes.Buffer{}
	r := io.TeeReader(in, buf)
	if limitHeader {
		r = io.LimitReader(r, 1<<20)
	}

	imgConfig, imgFormat, err := image.DecodeConfig(r)
	if err != nil {
		return 0, nil, 0, fmt.Errorf("%s: %w", err.Error(), ErrUnsupportedFormat)
	}

	// Check if image dimensions exceed maximum allowed size
	if imgConfig.Width <= 0 || imgConfig.Height <= 0 || imgConfig.Width > MaxImageWidth || imgConfig.Height > MaxImageHeight {
		return 0, nil, 0, fmt.Errorf("image dimensions %dx%d outside allowed maximum %dx%d: %w",
			imgConfig.Width, imgConfig.Height, MaxImageWidth, MaxImageHeight, ErrImageTooLarge)
	}

	format, err := ParseFormat(imgFormat)
	if err != nil {
		return 0, nil, 0, ErrUnsupportedFormat
	}

	return format, io.MultiReader(buf, in), imgConfig.Width * imgConfig.Height, nil
}

func getEmbeddedThumbnail(in io.Reader) ([]byte, io.Reader, error) {
	buf := &bytes.Buffer{}
	r := io.TeeReader(in, buf)
	wrappedReader := io.MultiReader(buf, in)

	offset := 0
	offsets := []int{12, 30}
	head := make([]byte, 0xffff)

	_, err := r.Read(head)
	if err != nil {
		return nil, wrappedReader, err
	}

	for _, offset = range offsets {
		if _, err = exif.ParseExifHeader(head[offset:]); err == nil {
			break
		}
	}

	if err != nil {
		return nil, wrappedReader, err
	}

	im, err := exifcommon.NewIfdMappingWithStandard()
	if err != nil {
		return nil, wrappedReader, err
	}

	_, index, err := exif.Collect(im, exif.NewTagIndex(), head[offset:])
	if err != nil {
		return nil, wrappedReader, err
	}

	ifd := index.RootIfd.NextIfd()
	if ifd == nil {
		return nil, wrappedReader, exif.ErrNoThumbnail
	}

	thm, err := ifd.Thumbnail()
	return thm, wrappedReader, err
}
