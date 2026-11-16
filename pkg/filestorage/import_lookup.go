package filestorage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type importedFileRecord struct {
	ID           string          `json:"id"`
	URL          string          `json:"url"`
	Size         int64           `json:"size"`
	Filename     string          `json:"filename"`
	OriginalName string          `json:"originalFilename"`
	Path         string          `json:"path"`
	ContentType  string          `json:"contentType"`
	Platform     string          `json:"platform"`
	ObjectID     string          `json:"objectId"`
	HashInfo     json.RawMessage `json:"hashInfo"`
}

// FindImportedURLAtPath recovers legacy completed imports through the original
// storage key. List filtering is fuzzy on older servers, so candidates must
// match the exact directory and original filename locally. Differing content
// or platforms is ambiguous and must never be published as the requested file.
func (c Client) FindImportedURLAtPath(ctx context.Context, storagePath, originalFilename string) (UploadResult, error) {
	storagePath, err := validatedImportLookupPath(storagePath, originalFilename)
	if err != nil {
		return UploadResult{}, err
	}
	originalFilename = strings.TrimSpace(originalFilename)
	var chosen *UploadResult
	var expectedTotal uint64
	for page := uint64(1); page <= 10; page++ {
		response, _, err := c.importAPI(ctx, http.MethodGet, "/api/v1/file/list", url.Values{
			"path": {storagePath}, "page": {strconv.FormatUint(page, 10)}, "size": {"100"},
		}, nil)
		if err != nil {
			return UploadResult{}, err
		}
		var list struct {
			Records []importedFileRecord `json:"records"`
			Total   *uint64              `json:"total"`
			Current *uint64              `json:"current"`
		}
		if json.Unmarshal(response.Data, &list) != nil || list.Records == nil || list.Total == nil || list.Current == nil ||
			*list.Current != page || len(list.Records) > 100 || *list.Total > 1000 {
			return UploadResult{}, &StorageError{Code: "response"}
		}
		if page == 1 {
			expectedTotal = *list.Total
		}
		if *list.Total != expectedTotal {
			return UploadResult{}, &StorageError{Code: "response", Retryable: true}
		}
		remaining := expectedTotal - min(expectedTotal, (page-1)*100)
		if uint64(len(list.Records)) != min(remaining, 100) {
			return UploadResult{}, &StorageError{Code: "response", Retryable: true}
		}
		for _, record := range list.Records {
			if record.Path != storagePath || record.OriginalName != originalFilename {
				continue
			}
			result, err := record.uploadResult()
			if err != nil {
				return UploadResult{}, err
			}
			if chosen != nil && (chosen.Size != result.Size || chosen.SHA256() != result.SHA256() ||
				chosen.Platform != result.Platform || chosen.ContentType != result.ContentType) {
				return UploadResult{}, &StorageError{Code: "digest"}
			}
			if chosen == nil {
				chosen = &result
			}
		}
		if page*100 >= expectedTotal {
			if chosen == nil {
				return UploadResult{}, ErrImportedFileNotFound
			}
			return *chosen, nil
		}
	}
	return UploadResult{}, &StorageError{Code: "response"}
}

func (r importedFileRecord) uploadResult() (UploadResult, error) {
	if strings.ContainsAny(r.URL, "\r\n\x00") {
		return UploadResult{}, &StorageError{Code: "locator"}
	}
	encoded := r.HashInfo
	if len(encoded) > 0 && encoded[0] == '"' {
		var nested string
		if json.Unmarshal(encoded, &nested) != nil {
			return UploadResult{}, &StorageError{Code: "response"}
		}
		encoded = []byte(nested)
	}
	var hashes UploadHashInfo
	if json.Unmarshal(encoded, &hashes) != nil {
		return UploadResult{}, &StorageError{Code: "hash"}
	}
	result := UploadResult{
		ID: r.ID, RawURL: strings.TrimSpace(r.URL), Size: r.Size, Filename: r.Filename, OriginalName: r.OriginalName,
		Path: r.Path, ContentType: strings.ToLower(strings.TrimSpace(strings.Split(r.ContentType, ";")[0])),
		Platform: r.Platform, ObjectID: r.ObjectID, HashInfo: hashes,
	}
	if strings.HasPrefix(r.URL, "https://") || strings.HasPrefix(r.URL, "http://") {
		result.URL = normalizePublicURL(r.URL)
	}
	if !validImportedMetadata(result) {
		return UploadResult{}, &StorageError{Code: "response"}
	}
	return result, nil
}
