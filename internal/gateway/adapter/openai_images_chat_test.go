package adapter

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/mirainya/Prism/internal/gateway/billing"
	"github.com/mirainya/Prism/internal/gateway/execution"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/internal/gateway/runtime"
)

func chatImageDispatch(operation string) repository.AsyncDispatch {
	return repository.AsyncDispatch{Method: http.MethodPost, Path: "/v1/chat/completions", VendorModel: "vendor-image",
		AdapterConfig: []byte(`{"chat_image":{"profile":"chat_completions_v1","operation":"` + operation + `"}}`)}
}

func TestChatImageRequestPinsModelAndConvertsReferences(t *testing.T) {
	for _, operation := range []string{ImagesGenerate, ImagesEdit} {
		t.Run(operation, func(t *testing.T) {
			input := OpenAIImagesRequest{Model: "public-image", Prompt: "draw a square", N: 1, ResponseFormat: "url", Size: "1024x1024"}
			if operation == ImagesEdit {
				input.ImageURLs = []string{"https://assets.example/ref.png"}
			}
			payload, _ := json.Marshal(input)
			loads := 0
			loader := runtime.AsyncAssetLoaderFunc(func(_ context.Context, location string) (runtime.AsyncAsset, error) {
				loads++
				if location != input.ImageURLs[0] {
					t.Fatalf("unexpected asset %q", location)
				}
				return runtime.AsyncAsset{Data: onePixelPNG, ContentType: "image/png"}, nil
			})
			fixed := chatImageDispatch(operation)
			if err := (AsyncOpenAIImages{}).ValidateSubmit(fixed, payload); err != nil {
				t.Fatal(err)
			}
			prepared, err := (AsyncOpenAIImages{}).PrepareWithAssets(context.Background(), "submit", fixed, payload, "", loader)
			if err != nil {
				t.Fatal(err)
			}
			var body struct {
				Model    string `json:"model"`
				Stream   bool   `json:"stream"`
				Messages []struct {
					Role    string `json:"role"`
					Content []struct {
						Type     string `json:"type"`
						Text     string `json:"text"`
						ImageURL struct {
							URL string `json:"url"`
						} `json:"image_url"`
					} `json:"content"`
				} `json:"messages"`
				GenerationConfig struct {
					ResponseModalities []string `json:"responseModalities"`
					ImageConfig        struct {
						AspectRatio string `json:"aspectRatio"`
					} `json:"imageConfig"`
				} `json:"generationConfig"`
			}
			if json.Unmarshal(prepared.Body, &body) != nil || body.Model != fixed.VendorModel || body.Stream || len(body.Messages) != 1 || body.Messages[0].Role != "user" || body.Messages[0].Content[0].Text != input.Prompt {
				t.Fatalf("invalid Chat request: %s", prepared.Body)
			}
			if strings.Join(body.GenerationConfig.ResponseModalities, ",") != "TEXT,IMAGE" || body.GenerationConfig.ImageConfig.AspectRatio != "1:1" {
				t.Fatal("missing image generation options")
			}
			if operation == ImagesEdit {
				if loads != 1 || len(body.Messages[0].Content) != 2 || body.Messages[0].Content[1].ImageURL.URL != "data:image/png;base64,"+base64.StdEncoding.EncodeToString(onePixelPNG) {
					t.Fatal("reference image was not preserved")
				}
			} else if loads != 0 || len(body.Messages[0].Content) != 1 {
				t.Fatal("generation downloaded an input")
			}
			if prepared.Path != fixed.Path || prepared.Header.Get("Content-Type") != "application/json" || prepared.CredentialHeader != "Authorization" || prepared.CredentialPrefix != "Bearer " {
				t.Fatal("invalid transport")
			}
		})
	}
}

func TestChatImageDecodesStringAndPartsIntoNormalDeliveries(t *testing.T) {
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(onePixelPNG)
	for _, content := range []any{
		"Here is the image: ![result](" + dataURL + ")",
		[]any{map[string]any{"type": "text", "text": "Done"}, map[string]any{"type": "image_url", "image_url": map[string]string{"url": dataURL}}},
	} {
		body, _ := json.Marshal(map[string]any{"created": 123, "choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": content}}}, "usage": map[string]int{"prompt_tokens": 12, "completion_tokens": 23, "total_tokens": 35}})
		payload := []byte(`{"model":"public-image","prompt":"draw","n":1,"response_format":"url"}`)
		observed, err := (AsyncOpenAIImages{}).DecodeWithDispatch("submit", chatImageDispatch(ImagesGenerate), payload, body)
		if err != nil {
			t.Fatal(err)
		}
		if observed.State != execution.AsyncSucceeded || observed.TaskID != "" || len(observed.Sources) != 1 || string(observed.Sources[0].InlineData) != string(onePixelPNG) || observed.Sources[0].ContentType != "image/png" {
			t.Fatalf("unexpected observation: %+v", observed)
		}
		if observed.Facts.Quantities[billing.QuantityGeneratedImages] != "1" || observed.Facts.Quantities[billing.QuantityInputTokens] != "12" || observed.Facts.Quantities[billing.QuantityOutputTokens] != "23" || !observed.Facts.Events[billing.ChargeSucceeded] {
			t.Fatal("incorrect billing facts")
		}
		if strings.Contains(string(observed.Result), base64.StdEncoding.EncodeToString(onePixelPNG)) {
			t.Fatal("image bytes leaked into normalized result")
		}
	}
}

func TestChatImageRejectsUnsupportedRequestsBeforeSubmission(t *testing.T) {
	for _, mutation := range []func(*OpenAIImagesRequest){
		func(in *OpenAIImagesRequest) { in.N = 2 },
		func(in *OpenAIImagesRequest) { in.Stream = true },
		func(in *OpenAIImagesRequest) { in.MaskURLs = []string{"https://assets.example/mask.png"} },
		func(in *OpenAIImagesRequest) { in.ImageURLs = []string{"https://assets.example/ref.png"} },
		func(in *OpenAIImagesRequest) { in.OutputFormat = "jpeg" },
		func(in *OpenAIImagesRequest) { in.Quality = "high" },
		func(in *OpenAIImagesRequest) { in.Size = "bad-size" },
		func(in *OpenAIImagesRequest) { in.AspectRatio = "100:1" },
	} {
		input := OpenAIImagesRequest{Model: "public-image", Prompt: "draw", N: 1, ResponseFormat: "url"}
		mutation(&input)
		payload, _ := json.Marshal(input)
		if err := (AsyncOpenAIImages{}).ValidateSubmit(chatImageDispatch(ImagesGenerate), payload); err == nil {
			t.Fatalf("accepted unsupported request: %s", payload)
		}
	}
	if err := (AsyncOpenAIImages{}).ValidateSubmit(chatImageDispatch(ImagesEdit), []byte(`{"model":"public-image","prompt":"edit","n":1,"response_format":"url"}`)); err == nil {
		t.Fatal("accepted edit without reference")
	}
}

func TestChatImageRejectsMissingInvalidAndTruncatedImages(t *testing.T) {
	valid := "data:image/png;base64," + base64.StdEncoding.EncodeToString(onePixelPNG)
	for _, example := range []struct{ content, finish string }{
		{"plain text only", "stop"},
		{"data:image/png;base64,bm90IGFuIGltYWdl", "stop"},
		{strings.Replace(valid, "image/png", "image/jpeg", 1), "stop"},
		{valid, "length"},
		{valid + " data:image/png;base64," + base64.StdEncoding.EncodeToString(append(onePixelPNG, 0)), "stop"},
	} {
		body, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"finish_reason": example.finish, "message": map[string]string{"content": example.content}}}})
		_, err := (AsyncOpenAIImages{}).DecodeWithDispatch("submit", chatImageDispatch(ImagesGenerate), []byte(`{"model":"public-image","prompt":"draw","n":1,"response_format":"url"}`), body)
		if err == nil {
			t.Fatal("accepted invalid Chat image response")
		}
	}
}

func TestChatImageCatalogRequiresExplicitTaskProfile(t *testing.T) {
	fixed := chatImageDispatch(ImagesGenerate)
	if err := ValidateOpenAIImagesCatalog(fixed.Method, fixed.Path, fixed.VendorModel, "task", fixed.AdapterConfig); err != nil {
		t.Fatal(err)
	}
	for _, in := range []struct {
		path, scope string
		config      []byte
	}{
		{"/v1/images/generations/async", "task", fixed.AdapterConfig},
		{fixed.Path, "request", fixed.AdapterConfig},
		{fixed.Path, "task", []byte(`{"chat_image":{"profile":"unknown","operation":"images.generate"}}`)},
		{fixed.Path, "task", []byte(`{"chat_image":{"profile":"chat_completions_v1","operation":"images.generate"},"async_image":{}}`)},
	} {
		if err := ValidateOpenAIImagesCatalog(fixed.Method, in.path, fixed.VendorModel, in.scope, in.config); err == nil {
			t.Fatal("accepted incompatible Chat image catalog")
		}
	}
	_, err := (AsyncOpenAIImages{}).Prepare(context.Background(), "query", fixed, nil, "provider-task")
	if err == nil {
		t.Fatal("synchronous Chat output cannot be polled upstream")
	}
}

func TestChatImageValidationIdentifiesUnsupportedCallerFields(t *testing.T) {
	for _, example := range []struct {
		param  string
		change func(*OpenAIImagesRequest)
	}{
		{"n", func(in *OpenAIImagesRequest) { in.N = 2 }},
		{"size", func(in *OpenAIImagesRequest) { in.Size = "2K" }},
		{"size", func(in *OpenAIImagesRequest) { in.Size = "1000x500" }},
		{"aspect_ratio", func(in *OpenAIImagesRequest) { in.AspectRatio = "2:1" }},
		{"quality", func(in *OpenAIImagesRequest) { in.Quality = "high" }},
		{"input_fidelity", func(in *OpenAIImagesRequest) { in.InputFidelity = "high" }},
		{"mask", func(in *OpenAIImagesRequest) { in.MaskURLs = []string{"https://assets.example/private-mask.png"} }},
		{"stream", func(in *OpenAIImagesRequest) { in.Stream = true }},
		{"output_format", func(in *OpenAIImagesRequest) { in.OutputFormat = "jpeg" }},
	} {
		t.Run(example.param, func(t *testing.T) {
			input := OpenAIImagesRequest{Model: "public-image", Prompt: "private prompt", N: 1,
				ResponseFormat: "url", ImageURLs: []string{"https://assets.example/private-image.png"}}
			example.change(&input)
			payload, _ := json.Marshal(input)
			err := (AsyncOpenAIImages{}).ValidateSubmit(chatImageDispatch(ImagesEdit), payload)
			var fieldErr *ImageRequestError
			if !errors.As(err, &fieldErr) || fieldErr.Param != example.param || !errors.Is(err, ErrInvalidImageRequest) {
				t.Fatalf("error = %v, want field %s", err, example.param)
			}
			if strings.Contains(err.Error(), input.Prompt) || strings.Contains(err.Error(), "assets.example") {
				t.Fatal("private request contents escaped in error")
			}
		})
	}
}
