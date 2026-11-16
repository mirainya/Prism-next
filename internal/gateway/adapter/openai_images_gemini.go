package adapter

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/jpeg"
	"image/png"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/internal/gateway/runtime"
)

type geminiImageConfig struct {
	Profile   string `json:"profile"`
	Operation string `json:"operation"`
}

func parseGeminiImageConfig(raw []byte) (geminiImageConfig, bool, error) {
	if len(raw) == 0 {
		return geminiImageConfig{}, false, nil
	}
	var envelope struct {
		GeminiImage *geminiImageConfig `json:"gemini_image"`
		ChatImage   json.RawMessage    `json:"chat_image"`
		AsyncImage  json.RawMessage    `json:"async_image"`
	}
	if json.Unmarshal(raw, &envelope) != nil {
		return geminiImageConfig{}, false, repository.ErrInvalidInput
	}
	if envelope.GeminiImage == nil {
		return geminiImageConfig{}, false, nil
	}
	config := *envelope.GeminiImage
	if config.Profile != "generate_content_v1" ||
		(config.Operation != ImagesGenerate && config.Operation != ImagesEdit) ||
		len(envelope.ChatImage) != 0 || len(envelope.AsyncImage) != 0 {
		return config, true, repository.ErrInvalidInput
	}
	return config, true, nil
}

func geminiImagePath(model string) string {
	return "/v1beta/models/" + url.PathEscape(model) + ":generateContent"
}

func validateGeminiImageSubmit(config geminiImageConfig, fixed repository.AsyncDispatch, payload []byte) (OpenAIImagesRequest, error) {
	if fixed.Method != http.MethodPost || strings.TrimSpace(fixed.VendorModel) == "" || fixed.Path != geminiImagePath(fixed.VendorModel) {
		return OpenAIImagesRequest{}, repository.ErrInvalidInput
	}
	input, err := decodeOpenAIImagesRequest(payload)
	if err != nil || validateOpenAIImagesRequest(config.Operation, input) != nil {
		return OpenAIImagesRequest{}, repository.ErrInvalidInput
	}
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
		{input.OutputFormat != "" && input.OutputFormat != "png" && input.OutputFormat != "jpeg", "output_format", "output_format must be omitted, png or jpeg for this model"},
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
	switch strings.ToUpper(strings.TrimSpace(input.Quality)) {
	case "", "AUTO":
		input.Quality = ""
	case "1K", "2K", "4K":
		input.Quality = strings.ToUpper(strings.TrimSpace(input.Quality))
	default:
		return OpenAIImagesRequest{}, invalidImageField("quality", "quality must be omitted, auto, 1K, 2K or 4K for this model")
	}
	input.Model = fixed.VendorModel
	return input, nil
}

func prepareGeminiImage(ctx context.Context, action string, config geminiImageConfig, fixed repository.AsyncDispatch, payload []byte, taskID string, loader ImageAssetLoader) (runtime.AsyncRequest, error) {
	if action != "submit" || taskID != "" {
		return runtime.AsyncRequest{}, repository.ErrInvalidInput
	}
	input, err := validateGeminiImageSubmit(config, fixed, payload)
	if err != nil {
		return runtime.AsyncRequest{}, err
	}
	parts := []any{map[string]string{"text": input.Prompt}}
	if config.Operation == ImagesEdit {
		if loader == nil {
			return runtime.AsyncRequest{}, repository.ErrInvalidInput
		}
		assets, err := loadImageAssets(ctx, input.ImageURLs, false, loader)
		if err != nil {
			return runtime.AsyncRequest{}, err
		}
		for _, asset := range assets {
			parts = append(parts, map[string]any{"inlineData": map[string]string{
				"mimeType": asset.ContentType, "data": base64.StdEncoding.EncodeToString(asset.Data),
			}})
		}
	}
	imageConfig := map[string]string{}
	if ratio, _ := chatImageAspectRatio(input); ratio != "" {
		imageConfig["aspectRatio"] = ratio
	}
	if input.Quality != "" {
		imageConfig["imageSize"] = input.Quality
	}
	configBody := map[string]any{"responseModalities": []string{"TEXT", "IMAGE"}}
	if len(imageConfig) != 0 {
		configBody["imageConfig"] = imageConfig
	}
	body, err := json.Marshal(map[string]any{
		"contents":         []any{map[string]any{"role": "user", "parts": parts}},
		"generationConfig": configBody,
	})
	if err != nil || len(body) > maxImageRequestBytes {
		return runtime.AsyncRequest{}, repository.ErrInvalidInput
	}
	return runtime.AsyncRequest{Method: fixed.Method, Path: fixed.Path, Body: body,
		Header: http.Header{"Content-Type": {"application/json"}}, CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
		MaxResponseBytes: maxImageRequestBytes}, nil
}

func decodeGeminiImageObservation(action string, config geminiImageConfig, fixed repository.AsyncDispatch, request, response []byte) (runtime.AsyncObservation, error) {
	if action != "submit" || len(response) == 0 || len(response) > maxImageRequestBytes {
		return runtime.AsyncObservation{}, ErrInvalidImageResponse
	}
	input, err := validateGeminiImageSubmit(config, fixed, request)
	if err != nil {
		return runtime.AsyncObservation{}, err
	}
	var envelope struct {
		Candidates []struct {
			FinishReason string `json:"finishReason"`
			Content      struct {
				Parts []struct {
					Thought    bool `json:"thought"`
					InlineData *struct {
						MIMEType string `json:"mimeType"`
						Data     string `json:"data"`
					} `json:"inlineData"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		Usage map[string]json.RawMessage `json:"usageMetadata"`
		Error json.RawMessage            `json:"error"`
	}
	if json.Unmarshal(response, &envelope) != nil || len(envelope.Candidates) != 1 ||
		len(envelope.Error) != 0 && string(envelope.Error) != "null" || envelope.Candidates[0].FinishReason != "STOP" {
		return runtime.AsyncObservation{}, ErrInvalidImageResponse
	}
	images := []chatInlineImage{}
	for _, part := range envelope.Candidates[0].Content.Parts {
		if part.Thought || part.InlineData == nil {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(part.InlineData.Data)
		if err != nil || len(data) == 0 || len(data) > maxImageOutputBytes || http.DetectContentType(data) != part.InlineData.MIMEType {
			return runtime.AsyncObservation{}, ErrInvalidImageResponse
		}
		if err := validateGeminiImageDimensions(input, data); err != nil {
			return runtime.AsyncObservation{}, err
		}
		encoded := part.InlineData.Data
		if input.OutputFormat != "" && outputFormatContentType(input.OutputFormat) != part.InlineData.MIMEType {
			decoded, _, err := image.Decode(bytes.NewReader(data))
			if err != nil {
				return runtime.AsyncObservation{}, ErrInvalidImageResponse
			}
			var converted bytes.Buffer
			if input.OutputFormat == "png" {
				err = png.Encode(&converted, decoded)
			} else {
				err = jpeg.Encode(&converted, decoded, &jpeg.Options{Quality: 90})
			}
			if err != nil || converted.Len() > maxImageOutputBytes {
				return runtime.AsyncObservation{}, ErrInvalidImageResponse
			}
			encoded = base64.StdEncoding.EncodeToString(converted.Bytes())
		}
		images = append(images, chatInlineImage{B64JSON: encoded})
	}
	if len(images) != 1 {
		return runtime.AsyncObservation{}, ErrInvalidImageResponse
	}
	usage := map[string]json.RawMessage{}
	for target, source := range map[string]string{"input_tokens": "promptTokenCount", "output_tokens": "candidatesTokenCount", "total_tokens": "totalTokenCount"} {
		if value, ok := envelope.Usage[source]; ok {
			usage[target] = value
		}
	}
	mapped, err := json.Marshal(struct {
		Data  []chatInlineImage          `json:"data"`
		Usage map[string]json.RawMessage `json:"usage"`
	}{images, usage})
	if err != nil {
		return runtime.AsyncObservation{}, err
	}
	return (OpenAIImages{}).DecodeObservation(mapped, false, input.OutputFormat)
}

// Gemini uses fixed pixel sizes for nominal aspect ratios. Allow rounding,
// but reject a different shape or resolution before recording billable success.
func validateGeminiImageDimensions(input OpenAIImagesRequest, data []byte) error {
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 || config.Width > 8192 || config.Height > 8192 {
		return ErrInvalidImageResponse
	}
	longest := max(config.Width, config.Height)
	switch input.Quality {
	case "1K":
		if longest < 1024 || longest >= 2048 {
			return ErrInvalidImageResponse
		}
	case "2K":
		if longest < 2048 || longest >= 4096 {
			return ErrInvalidImageResponse
		}
	case "4K":
		if longest < 4096 {
			return ErrInvalidImageResponse
		}
	}
	if ratio, _ := chatImageAspectRatio(input); ratio != "" {
		width, height, _ := strings.Cut(ratio, ":")
		w, _ := strconv.ParseFloat(width, 64)
		h, _ := strconv.ParseFloat(height, 64)
		if math.Abs(float64(config.Width)/float64(config.Height)/(w/h)-1) > 0.05 {
			return ErrInvalidImageResponse
		}
	}
	return nil
}
