package music

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"

	xdraw "golang.org/x/image/draw"

	"github.com/j0x3n/x-console/backend/internal/server/files"
)

// coverSizes are the long-edge sizes kept for every cover.
var coverSizes = []int{96, 256, 640}

const (
	maxCoverBytes  = 20 << 20
	maxCoverPixels = 50_000_000
)

func coverKey(hash string, size int) string {
	return fmt.Sprintf("covers/%s/%s/%d.jpg", hash[:2], hash, size)
}

// saveCover stores the three sizes of an image and returns its content hash.
// The same image is stored once, so every track of an album shares it.
func (m *Module) saveCover(ctx context.Context, data []byte) (string, error) {
	if len(data) == 0 || len(data) > maxCoverBytes {
		return "", errors.New("封面大小不对")
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	if _, err := m.store.Stat(ctx, coverKey(hash, coverSizes[len(coverSizes)-1])); err == nil {
		return hash, nil
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxCoverPixels {
		return "", errors.New("封面尺寸不对")
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	for _, size := range coverSizes {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, resizeCover(src, size), &jpeg.Options{Quality: 85}); err != nil {
			return "", err
		}
		if err := m.store.Put(ctx, coverKey(hash, size), &buf, int64(buf.Len())); err != nil {
			return "", err
		}
	}
	return hash, nil
}

// resizeCover scales the long edge down to size and flattens transparency on
// white. Images already smaller are not enlarged.
func resizeCover(src image.Image, size int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w > size || h > size {
		if w >= h {
			h, w = max(1, h*size/w), size
		} else {
			w, h = max(1, w*size/h), size
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, b, xdraw.Over, nil)
	return dst
}

// openCover opens the stored cover of the given size. ok is false when the
// image is missing.
func (m *Module) openCover(ctx context.Context, hash string, size int) (files.SeekReader, files.Info, bool, error) {
	rc, info, err := files.OpenSeeker(ctx, m.store, coverKey(hash, size))
	if errors.Is(err, files.ErrNotFound) {
		return nil, info, false, nil
	}
	return rc, info, err == nil, err
}
