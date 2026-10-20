package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/gshipman223/IconCraft/internal/core"
	"image"
	"image/draw"
	"image/png"
	"net/http"
	"path/filepath"
	"time"
)

type thumbRequest struct {
	app    core.App
	key    string
	result chan thumbResult
}
type thumbResult struct {
	data []byte
	err  error
}

func thumbKey(a core.App) string {
	// v2: previews are normalized 8-bit PNGs (see normalizeThumb); the new
	// key namespace orphans 16-bit previews cached by earlier builds.
	h := sha256.Sum256([]byte("v2:" + a.ID + ":" + core.PreviewStamp(a.Path)))
	return hex.EncodeToString(h[:])
}

// normalizeThumb re-encodes a native preview as a standard 8-bit PNG capped
// at 192px. The AppKit bridge emits 16-bit PNGs (~100KB each) that WebKit
// <img> elements fail to paint, leaving rows on the placeholder glyph; 8-bit
// previews are a fraction of the size and render everywhere. Applied-icon
// assets are never passed through here — their bytes stay exact.
func normalizeThumb(b []byte) ([]byte, error) {
	src, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w < 1 || h < 1 {
		return nil, errors.New("empty icon preview")
	}
	if w > 192 || h > 192 {
		scale := 192.0 / float64(max(w, h))
		w, h = max(1, int(float64(w)*scale)), max(1, int(float64(h)*scale))
	}
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	if bounds.Dx() == w && bounds.Dy() == h {
		draw.Draw(dst, dst.Bounds(), src, bounds.Min, draw.Src)
	} else {
		// Point-sampled downscale (stdlib only); native previews arrive at
		// 192px, so this path is rare and icon-sized.
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				sx := bounds.Min.X + x*bounds.Dx()/w
				sy := bounds.Min.Y + y*bounds.Dy()/h
				dst.Set(x, y, src.At(sx, sy))
			}
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, dst); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
func (s *Server) thumbnail(ctx context.Context, a core.App) ([]byte, error) {
	key := thumbKey(a)
	s.cacheMu.Lock()
	b := s.iconCache[key]
	s.cacheMu.Unlock()
	if b != nil {
		return b, nil
	}
	if disk, err := core.ReadLimited(filepath.Join(s.Engine.Dir, "previews", key+".png"), 2<<20); err == nil && core.ValidatePNG(disk) == nil {
		s.cacheIcon(key, disk)
		return disk, nil
	}
	s.thumbMu.Lock()
	if wait, ok := s.thumbWait[key]; ok {
		s.thumbMu.Unlock()
		select {
		case <-wait:
			s.cacheMu.Lock()
			b = s.iconCache[key]
			s.cacheMu.Unlock()
			if b == nil {
				return nil, errors.New("icon preview unavailable")
			}
			return b, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	wait := make(chan struct{})
	s.thumbWait[key] = wait
	s.thumbMu.Unlock()
	defer func() { s.thumbMu.Lock(); delete(s.thumbWait, key); close(wait); s.thumbMu.Unlock() }()
	req := thumbRequest{a, key, make(chan thumbResult, 1)}
	s.thumbOnce.Do(func() { go s.thumbnailLoop() })
	select {
	case s.thumbQueue <- req:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case v := <-req.result:
		return v.data, v.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (s *Server) thumbnailLoop() {
	for {
		select {
		case <-s.stop:
			return
		case first := <-s.thumbQueue:
			batch := []thumbRequest{first}
			timer := time.NewTimer(18 * time.Millisecond)
		collect:
			for len(batch) < 32 {
				select {
				case v := <-s.thumbQueue:
					batch = append(batch, v)
				case <-timer.C:
					break collect
				case <-s.stop:
					timer.Stop()
					return
				}
			}
			timer.Stop()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			var images map[string][]byte
			var err error
			if n, ok := s.Engine.Native.(interface {
				ExportThumbnails(context.Context, []string) (map[string][]byte, error)
			}); ok {
				paths := make([]string, 0, len(batch))
				for _, q := range batch {
					paths = append(paths, q.app.Path)
				}
				images, err = n.ExportThumbnails(ctx, paths)
			} else {
				images = map[string][]byte{}
				for _, q := range batch {
					b, er := s.Engine.Native.ExportIcon(ctx, q.app.Path, false)
					if er == nil {
						images[q.app.Path] = b
					} else {
						err = er
					}
				}
			}
			cancel()
			for _, q := range batch {
				b := images[q.app.Path]
				er := err
				if b != nil {
					if er = core.ValidatePNG(b); er == nil {
						b, er = normalizeThumb(b)
					}
				} else if er == nil {
					er = errors.New("app icon preview unavailable")
				}
				if er == nil {
					s.cacheIcon(q.key, b)
					_ = core.AtomicWrite(filepath.Join(s.Engine.Dir, "previews", q.key+".png"), b, 0600)
				}
				q.result <- thumbResult{b, er}
			}
		}
	}
}
func cacheHeaders(w http.ResponseWriter, r *http.Request, key string, immutable bool) bool {
	etag := fmt.Sprintf("\"%s\"", key)
	w.Header().Set("ETag", etag)
	if immutable {
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "private, max-age=0, must-revalidate")
	}
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	return false
}
