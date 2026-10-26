package adapter

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"strings"
	"testing"

	"github.com/mirainya/Prism/internal/gateway/billing"
	"github.com/mirainya/Prism/internal/gateway/execution"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/internal/gateway/runtime"
)

func geminiImageDispatch(operation string) repository.AsyncDispatch {
	return repository.AsyncDispatch{Method: http.MethodPost, Path: "/v1beta/models/vendor-image:generateContent", VendorModel: "vendor-image",
		AdapterConfig: []byte(`{"gemini_image":{"profile":"generate_content_v1","operation":"` + operation + `"}}`)}
}

func TestGeminiImageGeneratesAndEditsThroughExistingImageInput(t *testing.T) {
	for _, operation := range []string{ImagesGenerate, ImagesEdit} {
		t.Run(operation, func(t *testing.T) {
			input := OpenAIImagesRequest{Model: "public-image", Prompt: "draw a square", N: 1,
				ResponseFormat: "url", AspectRatio: "16:9", Quality: "2K"}
			if operation == ImagesEdit {
				input.ImageURLs = []string{"https://assets.example/ref.png"}
			}
			payload, _ := json.Marshal(input)
			loads := 0
			loader := runtime.AsyncAssetLoaderFunc(func(_ context.Context, location string) (runtime.AsyncAsset, error) {
				loads++
				if location != input.ImageURLs[0] {
					t.Fatalf("unexpected reference %q", location)
				}
				return runtime.AsyncAsset{Data: onePixelPNG, ContentType: "image/png"}, nil
			})
			fixed := geminiImageDispatch(operation)
			if err := (AsyncOpenAIImages{}).ValidateSubmit(fixed, payload); err != nil {
				t.Fatal(err)
			}
			prepared, err := (AsyncOpenAIImages{}).PrepareWithAssets(context.Background(), "submit", fixed, payload, "", loader)
			if err != nil {
				t.Fatal(err)
			}
			var body struct {
				Contents []struct {
					Role  string `json:"role"`
					Parts []struct {
						Text       string `json:"text"`
						InlineData struct {
							MIMEType string `json:"mimeType"`
							Data     string `json:"data"`
						} `json:"inlineData"`
					} `json:"parts"`
				} `json:"contents"`
				GenerationConfig struct {
					ResponseModalities []string `json:"responseModalities"`
					ImageConfig        struct {
						AspectRatio string `json:"aspectRatio"`
						ImageSize   string `json:"imageSize"`
					} `json:"imageConfig"`
				} `json:"generationConfig"`
			}
			if json.Unmarshal(prepared.Body, &body) != nil || len(body.Contents) != 1 || body.Contents[0].Role != "user" || body.Contents[0].Parts[0].Text != input.Prompt {
				t.Fatal("invalid native content")
			}
			if strings.Join(body.GenerationConfig.ResponseModalities, ",") != "TEXT,IMAGE" || body.GenerationConfig.ImageConfig.AspectRatio != "16:9" || body.GenerationConfig.ImageConfig.ImageSize != "2K" {
				t.Fatal("requested resolution or aspect ratio was lost")
			}
			if operation == ImagesEdit {
				if loads != 1 || len(body.Contents[0].Parts) != 2 || body.Contents[0].Parts[1].InlineData.Data != base64.StdEncoding.EncodeToString(onePixelPNG) || body.Contents[0].Parts[1].InlineData.MIMEType != "image/png" {
					t.Fatal("native reference bytes were not preserved")
				}
			} else if loads != 0 || len(body.Contents[0].Parts) != 1 {
				t.Fatal("generation loaded a reference")
			}
			if prepared.Path != fixed.Path || prepared.CredentialHeader != "Authorization" || prepared.CredentialPrefix != "Bearer " || prepared.Header.Get("Content-Type") != "application/json" || bytes.Contains(prepared.Body, []byte(input.Model)) {
				t.Fatal("native transport did not pin the configured model")
			}
			if prepared.MaxResponseBytes != maxImageRequestBytes {
				t.Fatal("native image response would be truncated by the metadata limit")
			}
		})
	}
}

func nativeImageResponse(t *testing.T, width, height int, finish string) []byte {
	t.Helper()
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewGray(image.Rect(0, 0, width, height))); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{
		"candidates": []any{map[string]any{"finishReason": finish, "content": map[string]any{"parts": []any{
			map[string]any{"text": "Done"},
			map[string]any{"thought": true, "inlineData": map[string]string{"mimeType": "image/png", "data": base64.StdEncoding.EncodeToString(onePixelPNG)}},
			map[string]any{"inlineData": map[string]string{"mimeType": "image/png", "data": base64.StdEncoding.EncodeToString(encoded.Bytes())}},
		}}}},
		"usageMetadata": map[string]int{"promptTokenCount": 12, "candidatesTokenCount": 23, "totalTokenCount": 35},
	})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestGeminiImageBillsOnlyACompletedImageWithRequestedDimensions(t *testing.T) {
	payload := []byte(`{"model":"public-image","prompt":"draw","n":1,"response_format":"url","quality":"2K","aspect_ratio":"16:9"}`)
	for _, example := range []struct {
		name          string
		width, height int
		finish        string
		valid         bool
	}{
		{"native_2k", 2752, 1536, "STOP", true},
		{"ignored_2k", 1376, 768, "STOP", false},
		{"ignored_ratio", 2048, 2048, "STOP", false},
		{"truncated", 2752, 1536, "MAX_TOKENS", false},
	} {
		t.Run(example.name, func(t *testing.T) {
			response := nativeImageResponse(t, example.width, example.height, example.finish)
			observed, err := (AsyncOpenAIImages{}).DecodeWithDispatch("submit", geminiImageDispatch(ImagesGenerate), payload, response)
			if !example.valid {
				if !errors.Is(err, ErrInvalidImageResponse) || observed.Facts.Events[billing.ChargeSucceeded] {
					t.Fatal("invalid size, shape or truncated result became billable success")
				}
				return
			}
			if err != nil || observed.State != execution.AsyncSucceeded || len(observed.Sources) != 1 || observed.Sources[0].ContentType != "image/png" || observed.TaskID != "" {
				t.Fatalf("invalid native observation: %v", err)
			}
			if observed.Facts.Quantities[billing.QuantityGeneratedImages] != "1" || observed.Facts.Quantities[billing.QuantityInputTokens] != "12" || observed.Facts.Quantities[billing.QuantityOutputTokens] != "23" || !observed.Facts.Events[billing.ChargeSucceeded] {
				t.Fatal("incorrect native billing facts")
			}
			if bytes.Contains(observed.Result, []byte("base64")) || bytes.Contains(observed.Result, []byte("inlineData")) {
				t.Fatal("native image bytes entered the persisted result")
			}
		})
	}
}

func TestGeminiImageRejectsUnsupportedControlsBeforeDownloadingReferences(t *testing.T) {
	for _, quality := range []string{"high", "low", "8K"} {
		payload, _ := json.Marshal(OpenAIImagesRequest{Model: "public-image", Prompt: "private prompt", N: 1, ResponseFormat: "url", Quality: quality, ImageURLs: []string{"https://assets.example/private.png"}})
		loads := 0
		loader := runtime.AsyncAssetLoaderFunc(func(context.Context, string) (runtime.AsyncAsset, error) {
			loads++
			return runtime.AsyncAsset{}, nil
		})
		_, err := (AsyncOpenAIImages{}).PrepareWithAssets(context.Background(), "submit", geminiImageDispatch(ImagesEdit), payload, "", loader)
		var fieldErr *ImageRequestError
		if !errors.As(err, &fieldErr) || fieldErr.Param != "quality" || loads != 0 || strings.Contains(err.Error(), "private") {
			t.Fatal("unsupported resolution was not safely rejected before loading inputs")
		}
	}
	// The new profile must not silently relax a Chat provider's resolution rules.
	payload := []byte(`{"model":"public-image","prompt":"draw","n":1,"response_format":"url","quality":"2K"}`)
	if err := (AsyncOpenAIImages{}).ValidateSubmit(chatImageDispatch(ImagesGenerate), payload); err == nil {
		t.Fatal("Chat profile accepted a native-only resolution")
	}
}

func TestGeminiImageHonorsRequestedPNGWhenProviderReturnsJPEG(t *testing.T) {
	var upstream bytes.Buffer
	if err := jpeg.Encode(&upstream, image.NewGray(image.Rect(0, 0, 16, 16)), nil); err != nil {
		t.Fatal(err)
	}
	response, _ := json.Marshal(map[string]any{"candidates": []any{map[string]any{
		"finishReason": "STOP", "content": map[string]any{"parts": []any{map[string]any{
			"inlineData": map[string]string{"mimeType": "image/jpeg", "data": base64.StdEncoding.EncodeToString(upstream.Bytes())},
		}}},
	}}})
	payload := []byte(`{"model":"public-image","prompt":"draw","n":1,"response_format":"url","output_format":"png"}`)
	observed, err := (AsyncOpenAIImages{}).DecodeWithDispatch("submit", geminiImageDispatch(ImagesGenerate), payload, response)
	if err != nil || len(observed.Sources) != 1 || observed.Sources[0].ContentType != "image/png" {
		t.Fatalf("native JPEG was not delivered in the requested format: %v", err)
	}
	if _, err := png.Decode(bytes.NewReader(observed.Sources[0].InlineData)); err != nil {
		t.Fatal("delivered PNG is invalid")
	}
}

func TestGeminiImageCatalogPinsProfileModelAndOperation(t *testing.T) {
	fixed := geminiImageDispatch(ImagesGenerate)
	if err := ValidateOpenAIImagesCatalog(fixed.Method, fixed.Path, fixed.VendorModel, "task", fixed.AdapterConfig); err != nil {
		t.Fatal(err)
	}
	for _, example := range []struct {
		path, scope string
		config      []byte
	}{
		{"/v1/chat/completions", "task", fixed.AdapterConfig},
		{"/v1beta/models/other-image:generateContent", "task", fixed.AdapterConfig},
		{fixed.Path, "request", fixed.AdapterConfig},
		{fixed.Path, "task", []byte(`{"gemini_image":{"profile":"unknown","operation":"images.generate"}}`)},
		{fixed.Path, "task", []byte(`{"gemini_image":{"profile":"generate_content_v1","operation":"images.generate"},"chat_image":{}}`)},
		{fixed.Path, "task", []byte(`{"gemini_image":{"profile":"generate_content_v1","operation":"images.generate"},"async_image":{}}`)},
	} {
		if err := ValidateOpenAIImagesCatalog(fixed.Method, example.path, fixed.VendorModel, example.scope, example.config); err == nil {
			t.Fatal("accepted incompatible native image catalog")
		}
	}
	if _, err := (AsyncOpenAIImages{}).Prepare(context.Background(), "query", fixed, nil, "provider-task"); err == nil {
		t.Fatal("native response was treated as an upstream polling task")
	}
}
