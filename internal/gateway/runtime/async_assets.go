package runtime

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/pkg/filestorage"
	"github.com/mirainya/Prism/pkg/safeurl"
)

const maxAsyncInputAssetBytes = 64 << 20

type storedAsyncAssetLoader struct {
	store  *repository.Store
	callID uint64
}

func (loader storedAsyncAssetLoader) LoadAsyncAsset(ctx context.Context, location string) (AsyncAsset, error) {
	return loader.loadAsyncAsset(ctx, location, DownloadRemoteImageInput)
}

func (loader storedAsyncAssetLoader) loadAsyncAsset(ctx context.Context, location string, readRemote func(context.Context, string) (AsyncAsset, error)) (AsyncAsset, error) {
	if loader.store == nil {
		return AsyncAsset{}, repository.ErrInvalidInput
	}
	if len(location) > 2048 {
		return readRemote(ctx, location)
	}
	storage, err := loader.store.ReadCallInputAssetStorage(ctx, loader.callID, location)
	if errors.Is(err, repository.ErrNotFound) && (strings.HasPrefix(location, "https://") || strings.HasPrefix(location, "http://")) {
		return readRemote(ctx, location)
	}
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) || errors.Is(err, repository.ErrInvalidInput) || errors.Is(err, repository.ErrConflict) {
			return AsyncAsset{}, repository.ErrConflict
		}
		return AsyncAsset{}, fmt.Errorf("%w: %v", ErrAsyncAssetUnavailable, err)
	}
	client := filestorage.DefaultClient()
	if storage.APIKey != "" {
		client = filestorage.WithBaseURLAndAPIKey(storage.XFSBaseURL, storage.APIKey)
	}
	objectLocator := storage.ObjectLocator
	if objectLocator == "" {
		objectLocator = location
	}
	data, storageErr := client.ReadURL(ctx, objectLocator, maxAsyncInputAssetBytes)
	if storageErr == nil {
		return AsyncAsset{Data: data, ContentType: http.DetectContentType(data)}, nil
	}
	if !strings.HasPrefix(location, "https://") && !strings.HasPrefix(location, "http://") {
		return AsyncAsset{}, fmt.Errorf("%w: %v", ErrAsyncAssetUnavailable, storageErr)
	}
	downloaded, downloadErr := safeurl.Download(ctx, location, maxAsyncInputAssetBytes)
	if downloadErr != nil {
		return AsyncAsset{}, fmt.Errorf("%w: %v", ErrAsyncAssetUnavailable, errors.Join(storageErr, downloadErr))
	}
	return AsyncAsset{Data: downloaded.Data, ContentType: downloaded.ContentType}, nil
}
