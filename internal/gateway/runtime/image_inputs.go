package runtime

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/mirainya/Prism/pkg/safeurl"
)

const maxRemoteImageInputBytes = 20 << 20

var errInvalidRemoteImageURL = errors.New("invalid image URL")

// ValidateRemoteImageInput checks the public address without reading or storing
// the image. URL-capable providers receive this original address.
func ValidateRemoteImageInput(ctx context.Context, location string) error {
	if len(location) == 0 || len(location) > 8192 || strings.TrimSpace(location) != location {
		return errInvalidRemoteImageURL
	}
	parsed, err := url.Parse(location)
	if err != nil || parsed.User != nil || parsed.Fragment != "" {
		return errInvalidRemoteImageURL
	}
	return safeurl.Validate(ctx, location)
}

// DownloadRemoteImageInput reads public input only when a provider needs file
// bytes. It does not send XFS credentials or create a managed storage object.
func DownloadRemoteImageInput(ctx context.Context, location string) (AsyncAsset, error) {
	return downloadRemoteImageInput(ctx, location, safeurl.Download)
}

func downloadRemoteImageInput(ctx context.Context, location string, download func(context.Context, string, int64) (*safeurl.Result, error)) (AsyncAsset, error) {
	if err := ValidateRemoteImageInput(ctx, location); err != nil {
		if !errors.Is(err, errInvalidRemoteImageURL) && !errors.Is(err, safeurl.ErrUnsafeURL) {
			return AsyncAsset{}, errors.Join(ErrAsyncAssetUnavailable, err)
		}
		return AsyncAsset{}, err
	}
	result, err := download(ctx, location, maxRemoteImageInputBytes)
	if err != nil {
		if errors.Is(err, safeurl.ErrUnsafeURL) {
			return AsyncAsset{}, err
		}
		return AsyncAsset{}, errors.Join(ErrAsyncAssetUnavailable, err)
	}
	detected := http.DetectContentType(result.Data)
	contentType := strings.TrimSpace(strings.Split(result.ContentType, ";")[0])
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = detected
	}
	if len(result.Data) == 0 || len(result.Data) > maxRemoteImageInputBytes || contentType != detected ||
		(detected != "image/png" && detected != "image/jpeg" && detected != "image/webp") {
		clear(result.Data)
		return AsyncAsset{}, errors.New("image must be PNG, JPEG, or WebP within 20 MiB")
	}
	return AsyncAsset{Data: result.Data, ContentType: contentType}, nil
}
