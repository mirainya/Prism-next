package delivery

import "encoding/json"

// protectedResultSource is an encrypted recovery envelope, not a public URL.
// No secret is included; credentials are resolved by the original Attempt.
type protectedResultSource struct {
	Version uint32              `json:"managed_result_version"`
	URL     string              `json:"url"`
	Auth    *UpstreamResultAuth `json:"upstream_auth"`
}

func EncodeManagedSource(source RemoteResult) []byte {
	if source.UpstreamAuth == nil {
		return []byte(source.URL)
	}
	raw, _ := json.Marshal(protectedResultSource{Version: 1, URL: source.URL, Auth: source.UpstreamAuth})
	return raw
}

func DecodeManagedSource(role string, raw []byte) (RemoteResult, error) {
	source := RemoteResult{Role: role, URL: string(raw)}
	if len(raw) > 0 && raw[0] == '{' {
		var saved protectedResultSource
		if err := json.Unmarshal(raw, &saved); err != nil || saved.Version != 1 || saved.Auth == nil {
			return RemoteResult{}, ErrInvalidResult
		}
		source.URL, source.UpstreamAuth = saved.URL, saved.Auth
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
