package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/internal/gateway/runtime"
)

func TestBackgroundImagesUseFixedVendorModelAndUpstreamFormat(t *testing.T) {
	fixed := repository.BackgroundCapabilityDispatch{
		CapabilityDispatch: repository.CapabilityDispatch{AdapterCode: "openai_images", AdapterVersion: 1, Method: "POST", Path: "/v1/images/generations", VendorModel: "gpt-image-2"},
		Operation:          ImagesGenerate,
	}
	in, err := (OpenAIImages{}).PrepareBackground(context.Background(), fixed,
		[]byte(`{"model":"gpt-image-2-2","prompt":"a blue square","response_format":"url","n":1}`),
		[]byte(`{"upstream_response_format":"b64_json"}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if json.Unmarshal(in.Prepared.Body, &wire) != nil || wire["model"] != "gpt-image-2" || wire["response_format"] != "b64_json" {
		t.Fatalf("unexpected provider request: %s", in.Prepared.Body)
	}
	if in.AdapterKey != OpenAIImagesAdapter || in.Prepared.Path != fixed.Path || in.Decode == nil {
		t.Fatalf("invalid background dispatch: %+v", in)
	}
}

type backgroundImageLoader struct{ loaded int }

func (l *backgroundImageLoader) LoadAsyncAsset(context.Context, string) (runtime.AsyncAsset, error) {
	l.loaded++
	return runtime.AsyncAsset{Data: append([]byte(nil), onePixelPNG...), ContentType: "image/png"}, nil
}

func TestBackgroundImageEditLoadsCommittedInput(t *testing.T) {
	fixed := repository.BackgroundCapabilityDispatch{CapabilityDispatch: repository.CapabilityDispatch{
		AdapterCode: "openai_images", AdapterVersion: 1, Method: "POST", Path: "/v1/images/edits", VendorModel: "vendor-edit",
	}, Operation: ImagesEdit}
	loader := &backgroundImageLoader{}
	in, err := (OpenAIImages{}).PrepareBackground(context.Background(), fixed,
		[]byte(`{"model":"public-edit","prompt":"edit","image_urls":["https://assets.example/input.png"],"n":1,"response_format":"url"}`), []byte(`{}`), loader)
	if err != nil {
		t.Fatal(err)
	}
	if loader.loaded != 1 || !bytes.Contains(in.Prepared.Body, onePixelPNG) || !bytes.Contains(in.Prepared.Body, []byte("vendor-edit")) {
		t.Fatal("background edit did not use committed asset and fixed model")
	}
}

func TestBackgroundImagesRejectStreamAndWrongAdapter(t *testing.T) {
	fixed := repository.BackgroundCapabilityDispatch{CapabilityDispatch: repository.CapabilityDispatch{
		AdapterCode: "openai_images", AdapterVersion: 1, Method: "POST", Path: "/v1/images/generations", VendorModel: "vendor",
	}, Operation: ImagesGenerate}
	for _, payload := range []string{`{"model":"public","prompt":"test","stream":true}`, `invalid`} {
		if _, err := (OpenAIImages{}).PrepareBackground(context.Background(), fixed, []byte(payload), []byte(`{}`), nil); err == nil {
			t.Fatal("invalid background request accepted")
		}
	}
	fixed.AdapterCode = "other"
	if _, err := (OpenAIImages{}).PrepareBackground(context.Background(), fixed, []byte(`{"model":"public","prompt":"test"}`), []byte(`{}`), nil); err == nil {
		t.Fatal("wrong adapter accepted")
	}
}
