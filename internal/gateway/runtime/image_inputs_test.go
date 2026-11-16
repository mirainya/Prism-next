package runtime

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/mirainya/Prism/pkg/safeurl"
)

func TestRemoteImageReadUsesOriginalAddressAndSizeLimit(t *testing.T) {
	image, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	location := "https://8.8.8.8/image.png?signature=original"
	asset, err := downloadRemoteImageInput(context.Background(), location, func(_ context.Context, url string, max int64) (*safeurl.Result, error) {
		if url != location || max != 20<<20 {
			t.Fatalf("download %s limit=%d", url, max)
		}
		return &safeurl.Result{Data: image, ContentType: "image/png"}, nil
	})
	if err != nil || string(asset.Data) != string(image) || asset.ContentType != "image/png" {
		t.Fatalf("asset=%+v err=%v", asset, err)
	}
}

func TestRemoteImageReadRejectsUnsafeLocationBeforeDownload(t *testing.T) {
	for _, location := range []string{"http://127.0.0.1/image", "http://10.0.0.1/image", "private/object", "https://key:secret@8.8.8.8/image"} {
		_, err := downloadRemoteImageInput(context.Background(), location, func(context.Context, string, int64) (*safeurl.Result, error) {
			t.Fatal("unsafe URL downloaded")
			return nil, nil
		})
		if err == nil {
			t.Fatalf("unsafe URL accepted: %s", location)
		}
	}
}

func TestRemoteImageReadRejectsSpoofedContentAndRetriesTransportFailures(t *testing.T) {
	_, err := downloadRemoteImageInput(context.Background(), "https://8.8.8.8/image", func(context.Context, string, int64) (*safeurl.Result, error) {
		return &safeurl.Result{Data: []byte("not an image"), ContentType: "image/png"}, nil
	})
	if err == nil || errors.Is(err, ErrAsyncAssetUnavailable) {
		t.Fatalf("spoofed image error=%v", err)
	}
	_, err = downloadRemoteImageInput(context.Background(), "https://8.8.8.8/image", func(context.Context, string, int64) (*safeurl.Result, error) {
		return nil, errors.New("connection reset")
	})
	if !errors.Is(err, ErrAsyncAssetUnavailable) {
		t.Fatalf("transport failure should remain retryable: %v", err)
	}
}
