package adapter

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/internal/gateway/runtime"
)

func (codec OpenAIImages) PrepareBackground(ctx context.Context, fixed repository.BackgroundCapabilityDispatch, payload, config []byte, loader runtime.AsyncAssetLoader) (runtime.CapabilityDispatchInput, error) {
	if fmt.Sprintf("%s@%d", fixed.AdapterCode, fixed.AdapterVersion) != OpenAIImagesAdapter {
		return runtime.CapabilityDispatchInput{}, repository.ErrConflict
	}
	var request OpenAIImagesRequest
	if json.Unmarshal(payload, &request) != nil || request.Stream {
		return runtime.CapabilityDispatchInput{}, repository.ErrInvalidInput
	}
	prepared, err := codec.PrepareWithConfig(ctx, fixed.Operation, fixed.Method, fixed.Path, fixed.VendorModel, payload, config, asyncImageAssetLoader{loader: loader})
	if err != nil {
		return runtime.CapabilityDispatchInput{}, err
	}
	return runtime.CapabilityDispatchInput{
		AdapterKey: OpenAIImagesAdapter,
		Prepared:   runtime.AsyncRequest{Method: prepared.Method, Path: prepared.Path, Body: prepared.Body, Header: prepared.Header},
		Decode: func(body []byte) (runtime.AsyncObservation, error) {
			return codec.DecodeObservation(body, prepared.Streaming, request.OutputFormat)
		},
	}, nil
}
