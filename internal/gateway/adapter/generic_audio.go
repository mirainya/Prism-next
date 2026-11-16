package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/mirainya/Prism/internal/gateway/billing"
	"github.com/mirainya/Prism/internal/gateway/delivery"
	"github.com/mirainya/Prism/internal/gateway/execution"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/internal/gateway/runtime"
	"github.com/mirainya/Prism/internal/video"
	"github.com/mirainya/Prism/internal/video/generic"
)

const (
	GenericAudioAdapter = "generic_audio@1"
	AudioGenerate       = "audio.generate"
)

type SpeechRequest struct {
	Model          string         `json:"model"`
	Input          string         `json:"input"`
	ReferenceAudio string         `json:"reference_audio,omitempty"`
	Emotion        *SpeechEmotion `json:"emotion,omitempty"`
}

type SpeechEmotion struct {
	Mode           string             `json:"mode,omitempty"`
	ReferenceAudio string             `json:"reference_audio,omitempty"`
	Random         *bool              `json:"random,omitempty"`
	Strengths      map[string]float64 `json:"strengths,omitempty"`
}

func DecodeSpeechRequest(reader io.Reader) (SpeechRequest, error) {
	var request SpeechRequest
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return request, fmt.Errorf("invalid audio request: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return request, fmt.Errorf("audio request must contain one JSON object")
	}
	return request, request.Normalize()
}

func (request *SpeechRequest) Normalize() error {
	request.Model = strings.TrimSpace(request.Model)
	request.ReferenceAudio = strings.TrimSpace(request.ReferenceAudio)
	if request.Model == "" || len(request.Model) > 128 || !utf8.ValidString(request.Input) || strings.TrimSpace(request.Input) == "" || strings.ContainsRune(request.Input, '\x00') {
		return fmt.Errorf("model and input are required")
	}
	if request.ReferenceAudio != "" && !delivery.ValidRemoteURL(request.ReferenceAudio) {
		return fmt.Errorf("reference_audio must be an HTTP(S) audio URL")
	}
	if emotion := request.Emotion; emotion != nil {
		switch emotion.Mode {
		case "", "voice", "reference", "vector":
		default:
			return fmt.Errorf("unsupported emotion mode")
		}
		if emotion.ReferenceAudio != "" && !delivery.ValidRemoteURL(emotion.ReferenceAudio) || emotion.Mode == "reference" && emotion.ReferenceAudio == "" {
			return fmt.Errorf("emotion.reference_audio must be an HTTP(S) audio URL")
		}
		for name, strength := range emotion.Strengths {
			switch name {
			case "happy", "angry", "sad", "afraid", "disgusted", "melancholic", "surprised", "calm":
			default:
				return fmt.Errorf("unsupported emotion strength")
			}
			if math.IsNaN(strength) || math.IsInf(strength, 0) || strength < 0 {
				return fmt.Errorf("emotion strengths must be nonnegative numbers")
			}
		}
	}
	return nil
}

func (request SpeechRequest) values(vendorModel string) map[string]any {
	values := map[string]any{"model": vendorModel, "input": request.Input}
	if request.ReferenceAudio != "" {
		values["reference_audio"] = request.ReferenceAudio
	}
	if emotion := request.Emotion; emotion != nil {
		if emotion.Mode != "" {
			values["emotion.mode"] = emotion.Mode
		}
		if emotion.ReferenceAudio != "" {
			values["emotion.reference_audio"] = emotion.ReferenceAudio
		}
		if emotion.Random != nil {
			values["emotion.random"] = *emotion.Random
		}
		for name, strength := range emotion.Strengths {
			values["emotion."+name] = strength
		}
	}
	return values
}

type audioMapping struct {
	URLPaths               []string `json:"url_paths"`
	FormatPaths            []string `json:"format_paths"`
	Format                 string   `json:"format"`
	InputMaxCharacters     int      `json:"input_max_characters"`
	ReferenceAudioRequired bool     `json:"reference_audio_required"`
	EmotionMaxStrength     *float64 `json:"emotion_max_strength"`
}

func audioCodec(fixed repository.AsyncDispatch) (*generic.RuntimeCodec, audioMapping, error) {
	var envelope struct {
		Audio audioMapping `json:"audio"`
	}
	if err := json.Unmarshal(fixed.AdapterConfig, &envelope); err != nil {
		return nil, audioMapping{}, repository.ErrInvalidInput
	}
	config := envelope.Audio
	if len(config.URLPaths) == 0 || config.Format == "" && len(config.FormatPaths) == 0 || config.Format != "" && !delivery.ValidAudioFormat(config.Format) || config.InputMaxCharacters < 0 || config.EmotionMaxStrength != nil && (*config.EmotionMaxStrength < 0 || math.IsInf(*config.EmotionMaxStrength, 0) || math.IsNaN(*config.EmotionMaxStrength)) {
		return nil, config, repository.ErrInvalidInput
	}
	for _, selector := range append(append([]string(nil), config.URLPaths...), config.FormatPaths...) {
		if strings.TrimSpace(selector) == "" {
			return nil, config, repository.ErrInvalidInput
		}
	}
	if err := generic.ValidateRuntimeCatalog(fixed.BaseURL, fixed.Method, fixed.Path, fixed.VendorModel, fixed.AdapterConfig); err != nil {
		return nil, config, err
	}
	codec, err := generic.NewRuntimeCodec(fixed.BaseURL, fixed.AdapterConfig)
	if err == nil && (!codec.MapsInput("input") || config.ReferenceAudioRequired && !codec.MapsInput("reference_audio")) {
		err = repository.ErrInvalidInput
	}
	return codec, config, err
}

func ValidateGenericAudioCatalog(baseURL, method, path, vendorModel, taskScope string, config []byte) error {
	if taskScope != "task" {
		return repository.ErrInvalidInput
	}
	_, _, err := audioCodec(repository.AsyncDispatch{BaseURL: baseURL, Method: method, Path: path, VendorModel: vendorModel, AdapterConfig: config})
	return err
}

type GenericAudio struct{}

func (GenericAudio) RequiresManagedCopy(fixed repository.AsyncDispatch) (bool, error) {
	_, _, err := audioCodec(fixed)
	return true, err
}

func (GenericAudio) Prepare(ctx context.Context, action string, fixed repository.AsyncDispatch, payload []byte, taskID string) (runtime.AsyncRequest, error) {
	codec, config, err := audioCodec(fixed)
	if err != nil {
		return runtime.AsyncRequest{}, err
	}
	if fixed.DeliveryMode != "managed_copy" {
		return runtime.AsyncRequest{}, repository.ErrInvalidInput
	}
	var prepared generic.RuntimeRequest
	switch action {
	case "submit":
		if taskID != "" {
			return runtime.AsyncRequest{}, repository.ErrInvalidInput
		}
		request, decodeErr := DecodeSpeechRequest(strings.NewReader(string(payload)))
		if decodeErr != nil {
			return runtime.AsyncRequest{}, decodeErr
		}
		if config.InputMaxCharacters > 0 && utf8.RuneCountInString(request.Input) > config.InputMaxCharacters || config.ReferenceAudioRequired && request.ReferenceAudio == "" {
			return runtime.AsyncRequest{}, fmt.Errorf("input does not meet model requirements")
		}
		if request.Emotion != nil && config.EmotionMaxStrength != nil {
			for _, strength := range request.Emotion.Strengths {
				if strength > *config.EmotionMaxStrength {
					return runtime.AsyncRequest{}, fmt.Errorf("emotion strength exceeds model limit")
				}
			}
		}
		prepared, err = codec.PrepareMappedSubmit(request.values(fixed.VendorModel))
	case "query":
		prepared, err = codec.PrepareQuery(taskID)
	default:
		return runtime.AsyncRequest{}, repository.ErrInvalidInput
	}
	if err != nil {
		return runtime.AsyncRequest{}, err
	}
	return runtime.AsyncRequest{Method: prepared.Method, Path: prepared.Path, Body: prepared.Body, Header: prepared.Header, CredentialHeader: prepared.CredentialHeader, CredentialPrefix: prepared.CredentialPrefix}, nil
}

func (GenericAudio) Decode(action string, request, response []byte) (runtime.AsyncObservation, error) {
	return runtime.AsyncObservation{}, repository.ErrInvalidInput
}

func (GenericAudio) DecodeWithDispatch(action string, fixed repository.AsyncDispatch, request, response []byte) (runtime.AsyncObservation, error) {
	codec, config, err := audioCodec(fixed)
	if err != nil {
		return runtime.AsyncObservation{}, err
	}
	decoded, err := codec.DecodeMappedTask(action, response, config.URLPaths, config.FormatPaths, config.Format)
	if err != nil {
		return runtime.AsyncObservation{}, err
	}
	out := runtime.AsyncObservation{TaskID: decoded.TaskID, Facts: billing.Facts{
		Events: map[billing.ChargeEvent]bool{billing.ChargeAccepted: true}, Quantities: map[billing.QuantitySource]string{},
		Expr: billing.ExprEnv{Declared: map[string]bool{"success": true, "duration_ms": true, "attempt_count": true}, Vars: map[string]billing.ExprValue{"success": billing.ExprNumber(billing.Zero())}},
	}}
	switch decoded.Status {
	case video.VideoTaskStatusQueued, video.VideoTaskStatusSubmitted:
		out.State = execution.AsyncAccepted
	case video.VideoTaskStatusTracking:
		out.State = execution.AsyncRunning
	case video.VideoTaskStatusFailed:
		out.State = execution.AsyncFailed
	case video.VideoTaskStatusCancelled:
		out.State = execution.AsyncCancelled
	case video.VideoTaskStatusCompleted:
		out.State = execution.AsyncSucceeded
		if !delivery.ValidAudioFormat(decoded.Format) {
			return out, delivery.ErrInvalidResult
		}
		one, _ := billing.ParseAmount("1", 0, true)
		out.Facts.Expr.Vars["success"] = billing.ExprNumber(one)
		out.Sources = []delivery.RemoteResult{{Role: "audio", URL: decoded.URL, AudioFormat: decoded.Format}}
		if header, prefix, protected := codec.ResultAuthentication(); protected {
			out.Sources[0].UpstreamAuth = &delivery.UpstreamResultAuth{Header: header, Prefix: prefix}
		}
		if !delivery.ValidSource(out.Sources[0]) {
			return out, delivery.ErrInvalidResult
		}
		out.Result, err = json.Marshal(delivery.AudioResult{SchemaVersion: delivery.ResultSchemaVersion, Kind: delivery.AudioResultKind, Format: decoded.Format})
	default:
		return out, repository.ErrInvalidInput
	}
	if out.State == execution.AsyncFailed {
		out.ProviderErrorCode = "audio_task_failed"
		out.ProviderErrorMessage = "audio generation failed"
	}
	if action == "submit" && out.TaskID == "" && out.State != execution.AsyncFailed {
		return out, repository.ErrInvalidInput
	}
	return out, err
}

func genericAudioManifest() Manifest {
	return Manifest{Adapter: "generic_audio", AdapterVersion: 1, Capability: "audio", DownstreamPaths: []string{"/v1/audio/generations"},
		BillingVars: map[varLayer][]BillingVar{LayerCommon: commonBillingVars()},
		Variants:    []Variant{{Code: DefaultVariant, Display: "通用语音", ModelsFromConfig: true, PricingHints: PricingHints{DefaultMode: "flat", Example: "0.10"}}}}
}

var _ runtime.AsyncCodec = GenericAudio{}
