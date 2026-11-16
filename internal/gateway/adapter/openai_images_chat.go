package adapter

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/internal/gateway/runtime"
)

// Chat image support is explicit product configuration. Model names never
// select a protocol, and existing Images declarations keep their own codec.
type chatImageConfig struct {
	Profile   string `json:"profile"`
	Operation string `json:"operation"`
}

func parseChatImageConfig(raw []byte) (chatImageConfig, bool, error) {
	if len(raw) == 0 {
		return chatImageConfig{}, false, nil
	}
	var envelope struct {
		ChatImage  *chatImageConfig `json:"chat_image"`
		AsyncImage json.RawMessage  `json:"async_image"`
	}
	if json.Unmarshal(raw, &envelope) != nil {
		return chatImageConfig{}, false, repository.ErrInvalidInput
	}
	if envelope.ChatImage == nil {
		return chatImageConfig{}, false, nil
	}
	config := *envelope.ChatImage
	if config.Profile != "chat_completions_v1" ||
		(config.Operation != ImagesGenerate && config.Operation != ImagesEdit) ||
		len(envelope.AsyncImage) != 0 {
		return config, true, repository.ErrInvalidInput
	}
	return config, true, nil
}

func validateChatImageSubmit(config chatImageConfig, fixed repository.AsyncDispatch, payload []byte) (OpenAIImagesRequest, error) {
	if fixed.Method != http.MethodPost || fixed.Path != "/v1/chat/completions" || strings.TrimSpace(fixed.VendorModel) == "" {
		return OpenAIImagesRequest{}, repository.ErrInvalidInput
	}
	input, err := decodeOpenAIImagesRequest(payload)
	if err != nil || validateOpenAIImagesRequest(config.Operation, input) != nil {
		return OpenAIImagesRequest{}, repository.ErrInvalidInput
	}
	// Reject unsupported controls before a billable exchange, identifying the
	// caller field without exposing provider or request contents.
	checks := []struct {
		invalid bool
		param   string
		message string
	}{
		{input.N != 1, "n", "n must be 1 for this model"},
		{input.Stream, "stream", "stream must be false for this model"},
		{input.PartialImages != nil, "partial_images", "partial_images is not supported by this model"},
		{len(input.MaskURLs) != 0, "mask", "mask is not supported by this model"},
		{input.OutputCompression != nil, "output_compression", "output_compression is not supported by this model"},
		{input.OutputFormat != "" && input.OutputFormat != "png", "output_format", "output_format must be omitted or png for this model"},
		{input.Quality != "" && input.Quality != "auto", "quality", "quality must be omitted or auto for this model"},
		{input.Style != "", "style", "style is not supported by this model"},
		{input.Background != "" && input.Background != "auto", "background", "background must be omitted or auto for this model"},
		{input.Moderation != "", "moderation", "moderation is not supported by this model"},
		{input.InputFidelity != "", "input_fidelity", "input_fidelity is not supported by this model"},
	}
	for _, check := range checks {
		if check.invalid {
			return OpenAIImagesRequest{}, invalidImageField(check.param, check.message)
		}
	}
	if _, err := chatImageAspectRatio(input); err != nil {
		return OpenAIImagesRequest{}, err
	}
	input.Model = fixed.VendorModel
	return input, nil
}

func chatImageAspectRatio(input OpenAIImagesRequest) (string, error) {
	ratio := input.AspectRatio
	if input.Size != "" && input.Size != "auto" {
		width, height, ok := strings.Cut(input.Size, "x")
		w, we := strconv.Atoi(width)
		h, he := strconv.Atoi(height)
		if !ok || we != nil || he != nil || w <= 0 || h <= 0 || w > 8192 || h > 8192 {
			return "", invalidImageField("size", "size must be auto or WIDTHxHEIGHT for this model, for example 1024x1024")
		}
		a, b := w, h
		for b != 0 {
			a, b = b, a%b
		}
		ratio = strconv.Itoa(w/a) + ":" + strconv.Itoa(h/a)
	}
	switch ratio {
	case "", "1:1", "2:3", "3:2", "3:4", "4:3", "4:5", "5:4", "9:16", "16:9", "21:9":
		return ratio, nil
	default:
		param := "aspect_ratio"
		if input.Size != "" && input.Size != "auto" {
			param = "size"
		}
		return "", invalidImageField(param, param+" must use a supported image aspect ratio: 1:1, 2:3, 3:2, 3:4, 4:3, 4:5, 5:4, 9:16, 16:9 or 21:9")
	}
}

func prepareChatImage(ctx context.Context, action string, config chatImageConfig, fixed repository.AsyncDispatch, payload []byte, taskID string, loader ImageAssetLoader) (runtime.AsyncRequest, error) {
	if action != "submit" || taskID != "" {
		return runtime.AsyncRequest{}, repository.ErrInvalidInput
	}
	input, err := validateChatImageSubmit(config, fixed, payload)
	if err != nil {
		return runtime.AsyncRequest{}, err
	}
	parts := []any{map[string]any{"type": "text", "text": input.Prompt}}
	if config.Operation == ImagesEdit {
		if loader == nil {
			return runtime.AsyncRequest{}, repository.ErrInvalidInput
		}
		assets, err := loadImageAssets(ctx, input.ImageURLs, false, loader)
		if err != nil {
			return runtime.AsyncRequest{}, err
		}
		for _, asset := range assets {
			parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]string{
				"url": "data:" + asset.ContentType + ";base64," + base64.StdEncoding.EncodeToString(asset.Data),
			}})
		}
	}
	configBody := map[string]any{"responseModalities": []string{"TEXT", "IMAGE"}}
	ratio, _ := chatImageAspectRatio(input)
	if ratio != "" {
		configBody["imageConfig"] = map[string]string{"aspectRatio": ratio}
	}
	body, err := json.Marshal(map[string]any{
		"model": input.Model, "stream": false,
		"messages":         []any{map[string]any{"role": "user", "content": parts}},
		"generationConfig": configBody,
	})
	if err != nil || len(body) > maxImageRequestBytes {
		return runtime.AsyncRequest{}, repository.ErrInvalidInput
	}
	return runtime.AsyncRequest{Method: fixed.Method, Path: fixed.Path, Body: body,
		Header:           http.Header{"Content-Type": {"application/json"}},
		CredentialHeader: "Authorization", CredentialPrefix: "Bearer "}, nil
}

var chatImageDataURL = regexp.MustCompile(`data:(image/(?:png|jpeg|webp));base64,([A-Za-z0-9+/=]+)`)

type chatInlineImage struct {
	B64JSON string `json:"b64_json"`
}

func decodeChatImageObservation(action string, config chatImageConfig, fixed repository.AsyncDispatch, request, response []byte) (runtime.AsyncObservation, error) {
	if action != "submit" || len(response) == 0 || len(response) > maxImageRequestBytes {
		return runtime.AsyncObservation{}, ErrInvalidImageResponse
	}
	input, err := validateChatImageSubmit(config, fixed, request)
	if err != nil {
		return runtime.AsyncObservation{}, err
	}
	var envelope struct {
		Created int64 `json:"created"`
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage map[string]json.RawMessage `json:"usage"`
		Error json.RawMessage            `json:"error"`
	}
	if json.Unmarshal(response, &envelope) != nil || len(envelope.Choices) != 1 ||
		len(envelope.Error) != 0 && string(envelope.Error) != "null" || envelope.Choices[0].FinishReason != "stop" {
		return runtime.AsyncObservation{}, ErrInvalidImageResponse
	}
	content := envelope.Choices[0].Message.Content
	var text string
	var texts []string
	if json.Unmarshal(content, &text) == nil {
		texts = append(texts, text)
	} else {
		var parts []struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			ImageURL struct {
				URL string `json:"url"`
			} `json:"image_url"`
		}
		if json.Unmarshal(content, &parts) != nil {
			return runtime.AsyncObservation{}, ErrInvalidImageResponse
		}
		for _, part := range parts {
			switch part.Type {
			case "text":
				texts = append(texts, part.Text)
			case "image_url":
				texts = append(texts, part.ImageURL.URL)
			}
		}
	}
	images := []chatInlineImage{}
	seen := map[string]bool{}
	for _, text := range texts {
		for _, match := range chatImageDataURL.FindAllStringSubmatch(text, -1) {
			if seen[match[2]] {
				continue
			}
			seen[match[2]] = true
			data, err := base64.StdEncoding.DecodeString(match[2])
			if err != nil || len(data) == 0 || len(data) > maxImageOutputBytes || http.DetectContentType(data) != match[1] {
				return runtime.AsyncObservation{}, ErrInvalidImageResponse
			}
			images = append(images, chatInlineImage{B64JSON: match[2]})
		}
	}
	if len(images) != 1 {
		return runtime.AsyncObservation{}, ErrInvalidImageResponse
	}
	usage := map[string]json.RawMessage{}
	for target, source := range map[string]string{"input_tokens": "prompt_tokens", "output_tokens": "completion_tokens", "total_tokens": "total_tokens"} {
		if value, ok := envelope.Usage[source]; ok {
			usage[target] = value
		}
	}
	// Reuse the existing result, delivery and billing normalization. Inline
	// bytes are imported by the normal token-bound storage pipeline.
	mapped, err := json.Marshal(struct {
		Created int64                      `json:"created"`
		Data    []chatInlineImage          `json:"data"`
		Usage   map[string]json.RawMessage `json:"usage"`
	}{envelope.Created, images, usage})
	if err != nil {
		return runtime.AsyncObservation{}, err
	}
	return (OpenAIImages{}).DecodeObservation(mapped, false, input.OutputFormat)
}
