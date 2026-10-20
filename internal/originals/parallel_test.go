package originals

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSlowPrimaryCannotBlockIndependentFallback(t *testing.T) {
	r := New()
	var started atomic.Int32
	r.Client.Transport = transport(func(q *http.Request) (*http.Response, error) {
		if strings.Contains(q.URL.Path, "products/") {
			started.Add(1)
			<-q.Context().Done()
			return nil, q.Context().Err()
		}
		if strings.HasSuffix(q.URL.Path, "photoshop-40.svg") {
			return response(200, []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64"/>`)), nil
		}
		return response(404, nil), nil
	})
	start := time.Now()
	c, err := r.Resolve(context.Background(), "photocraft", nil)
	if err != nil || c.Source == "" || time.Since(start) > time.Second {
		t.Fatal(time.Since(start), err)
	}
	if started.Load() == 0 {
		t.Fatal("primary was never checked")
	}
}
func TestCurrentPageBeatsFixedFallbackWithinGrace(t *testing.T) {
	r := New()
	r.Client.Transport = transport(func(q *http.Request) (*http.Response, error) {
		if q.URL.Path == "/products/photoshop.html" {
			select {
			case <-time.After(30 * time.Millisecond):
			case <-q.Context().Done():
				return nil, q.Context().Err()
			}
			return response(200, []byte(`<img alt="Photoshop product icon" src="/new-current.png">`)), nil
		}
		if q.URL.Path == "/new-current.png" {
			return response(200, testPNG(64, 64)), nil
		}
		if strings.HasSuffix(q.URL.Path, ".svg") {
			return response(200, []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64"/>`)), nil
		}
		return response(404, nil), nil
	})
	c, err := r.Resolve(context.Background(), "photocraft", nil)
	if err != nil || !strings.HasSuffix(c.Source, "new-current.png") {
		t.Fatal(c.Source, err)
	}
}
func TestConditionalHTTPReusesUnchangedOriginalBytes(t *testing.T) {
	r := New()
	var calls atomic.Int32
	r.Client.Transport = transport(func(q *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			v := response(200, testPNG(64, 64))
			v.Header.Set("ETag", "v1")
			return v, nil
		}
		if q.Header.Get("If-None-Match") != "v1" {
			t.Error("no conditional request")
		}
		return response(304, nil), nil
	})
	a, _ := r.get(context.Background(), "https://www.adobe.com/icon.png", maxImage)
	b, err := r.get(context.Background(), "https://www.adobe.com/icon.png", maxImage)
	if err != nil || string(a) != string(b) {
		t.Fatal("304 lost data", err)
	}
}
func TestKnownProductsHaveIndependentAlternatives(t *testing.T) {
	for id, p := range Products() {
		if len(p.Direct)+len(p.ExtraPages) == 0 {
			t.Fatal("only one source", id)
		}
	}
}
