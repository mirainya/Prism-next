package delivery

import "encoding/json"

// protectedResultSource is an encrypted recovery envelope, not a public URL.
// No secret is included; credentials are resolved by the original Attempt.
type protectedResultSource struct {
	Version     uint32              `json:"managed_result_version"`
	URL         string              `json:"url"`
	Auth        *UpstreamResultAuth `json:"upstream_auth"`
	Role        string              `json:"role,omitempty"`
	AudioFormat string              `json:"audio_format,omitempty"`
}

func EncodeManagedSource(source RemoteResult) []byte {
	if source.UpstreamAuth == nil && source.Role != "audio" {
		return []byte(source.URL)
	}
	role := ""
	if source.Role == "audio" {
		role = "audio"
	}
	raw, _ := json.Marshal(protectedResultSource{Version: 1, URL: source.URL, Auth: source.UpstreamAuth, Role: role, AudioFormat: source.AudioFormat})
	return raw
}

func DecodeManagedSource(role string, raw []byte) (RemoteResult, error) {
	source := RemoteResult{Role: role, URL: string(raw)}
	if len(raw) > 0 && raw[0] == '{' {
		var saved protectedResultSource
		if err := json.Unmarshal(raw, &saved); err != nil || saved.Version != 1 || saved.Auth == nil && saved.Role != "audio" || saved.Role != "" && saved.Role != "audio" {
			return RemoteResult{}, ErrInvalidResult
		}
		source.URL, source.UpstreamAuth = saved.URL, saved.Auth
		if saved.Role != "" {
			source.Role = saved.Role
		}
		if saved.AudioFormat != "" && (source.Role != "audio" || !ValidAudioFormat(saved.AudioFormat)) {
			return RemoteResult{}, ErrInvalidResult
		}
		source.AudioFormat = saved.AudioFormat
	}
	if !ValidSource(source) {
		return RemoteResult{}, ErrInvalidResult
	}
	return source, nil
}

func ValidManagedSource(raw []byte) bool {
	_, err := DecodeManagedSource("video", raw)
	return err == nil
}
