package delivery

import "encoding/json"

const AudioResultKind = "audio"

func ValidAudioFormat(format string) bool {
	switch format {
	case "wav", "mp3", "ogg", "opus", "flac", "m4a", "aac", "pcm":
		return true
	default:
		return false
	}
}

type AudioResult struct {
	SchemaVersion uint32 `json:"schema_version"`
	Kind          string `json:"kind"`
	Format        string `json:"format"`
	DeliveryID    uint64 `json:"delivery_id,string,omitempty"`
}

func isAudioResult(plaintext []byte) bool {
	var result struct {
		Kind string `json:"kind"`
	}
	return json.Unmarshal(plaintext, &result) == nil && result.Kind == AudioResultKind
}

func validateAudioResult(plaintext []byte, sources []RemoteResult) error {
	var result AudioResult
	if json.Unmarshal(plaintext, &result) != nil || result.SchemaVersion != ResultSchemaVersion || result.Kind != AudioResultKind || !ValidAudioFormat(result.Format) || result.DeliveryID != 0 || len(sources) != 1 || sources[0].Role != "audio" || !ValidSource(sources[0]) {
		return ErrInvalidResult
	}
	return nil
}
