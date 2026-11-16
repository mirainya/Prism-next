package filestorage

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

func managedImportTestRecord() importedFileRecord {
	result := managedImportTestResult()
	hashes, _ := json.Marshal(result.HashInfo)
	return importedFileRecord{
		ID: "file-1", URL: result.RawURL, Size: result.Size, Filename: result.Filename,
		OriginalName: result.OriginalName, Path: result.Path, ContentType: result.ContentType,
		Platform: result.Platform, HashInfo: hashes,
	}
}

func writeManagedImportList(w http.ResponseWriter, records []importedFileRecord, total, current int) {
	writeManagedImportResponse(w, map[string]any{"records": records, "total": total, "current": current})
}

func TestManagedURLImportLookupUsesExactTargetAndBoundKey(t *testing.T) {
	var calls atomic.Int32
	const storagePath = "prism/video_1%2/"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/file/list" || r.Header.Get("X-Api-Key") != "bound-storage-key" ||
			r.URL.Query().Get("path") != storagePath || r.URL.Query().Get("page") != "1" || r.URL.Query().Get("size") != "100" {
			t.Error("lookup did not preserve the bound credential and exact directory")
		}
		actual := managedImportTestRecord()
		actual.Path = storagePath
		sibling, prefix, suffix := actual, actual, actual
		sibling.Path, prefix.OriginalName, suffix.OriginalName = storagePath+"sibling/", "prefix-"+actual.OriginalName, actual.OriginalName+".bak"
		// Malformed unrelated metadata is irrelevant; fuzzy matches are not candidates.
		sibling.HashInfo, prefix.HashInfo, suffix.HashInfo = nil, nil, nil
		writeManagedImportList(w, []importedFileRecord{sibling, prefix, suffix, actual}, 4, 1)
	}))
	defer server.Close()
	result, err := WithBaseURLAndAPIKey(server.URL, "bound-storage-key").FindImportedURLAtPath(context.Background(), "/"+storagePath, " "+managedImportTestName+" ")
	if err != nil || result.Path != storagePath || result.OriginalName != managedImportTestName || result.SHA256() != strings.Repeat("a", 64) {
		t.Fatalf("exact lookup failed: %v", err)
	}
	if result.URL != "" || result.RawURL != managedImportTestResult().RawURL || calls.Load() != 1 {
		t.Fatal("relative locator became a browser URL or lookup performed an unexpected request")
	}
}

func TestManagedURLImportLookupAcceptsHashFormatsAndIdenticalDuplicates(t *testing.T) {
	for _, hashString := range []bool{false, true} {
		t.Run(strconv.FormatBool(hashString), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				first := managedImportTestRecord()
				first.URL, first.ContentType = "http://storage.example/first.mp4", "VIDEO/MP4; charset=binary"
				if hashString {
					first.HashInfo, _ = json.Marshal(string(first.HashInfo))
				}
				second := first
				second.ID, second.Filename, second.URL = "file-2", "second.mp4", "http://storage.example/second.mp4"
				second.ContentType = "video/mp4"
				writeManagedImportList(w, []importedFileRecord{first, second}, 2, 1)
			}))
			defer server.Close()
			got, err := WithBaseURLAndAPIKey(server.URL, "bound-storage-key").FindImportedURLAtPath(context.Background(), managedImportTestPath, managedImportTestName)
			if err != nil || got.ID != "file-1" || got.URL != "https://storage.example/first.mp4" || got.RawURL != "http://storage.example/first.mp4" || got.ContentType != "video/mp4" {
				t.Fatalf("duplicate reconciliation failed: %v", err)
			}
		})
	}
}

func TestManagedURLImportLookupRejectsConflictingDuplicates(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*importedFileRecord)
	}{
		{"size", func(v *importedFileRecord) { v.Size++ }},
		{"sha256", func(v *importedFileRecord) {
			v.HashInfo = json.RawMessage(`{"sha256":"` + strings.Repeat("b", 64) + `"}`)
		}},
		{"platform", func(v *importedFileRecord) { v.Platform = "another-platform" }},
		{"mime", func(v *importedFileRecord) { v.ContentType = "image/png" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				first, second := managedImportTestRecord(), managedImportTestRecord()
				tc.mutate(&second)
				writeManagedImportList(w, []importedFileRecord{first, second}, 2, 1)
			}))
			defer server.Close()
			_, err := WithBaseURLAndAPIKey(server.URL, "bound-storage-key").FindImportedURLAtPath(context.Background(), managedImportTestPath, managedImportTestName)
			requireManagedImportError(t, err, "digest")
		})
	}
}

func TestManagedURLImportLookupMissingFileNeverUploads(t *testing.T) {
	for _, empty := range []bool{true, false} {
		t.Run(strconv.FormatBool(empty), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/file/list" {
					t.Error("missing file triggered a write")
				}
				records := []importedFileRecord{}
				if !empty {
					record := managedImportTestRecord()
					record.OriginalName += ".bak"
					records = append(records, record)
				}
				writeManagedImportList(w, records, len(records), 1)
			}))
			defer server.Close()
			_, err := WithBaseURLAndAPIKey(server.URL, "bound-storage-key").FindImportedURLAtPath(context.Background(), managedImportTestPath, managedImportTestName)
			if !errors.Is(err, ErrImportedFileNotFound) || calls.Load() != 1 {
				t.Fatalf("calls=%d error=%v", calls.Load(), err)
			}
		})
	}
}

func TestManagedURLImportLookupRejectsIncompleteListing(t *testing.T) {
	for _, tc := range []struct {
		name string
		data any
	}{
		{"missing_total", map[string]any{"records": []any{}, "current": 1}},
		{"missing_current", map[string]any{"records": []any{}, "total": 0}},
		{"null_records", map[string]any{"records": nil, "total": 0, "current": 1}},
		{"wrong_page", map[string]any{"records": []any{}, "total": 0, "current": 2}},
		{"truncated", map[string]any{"records": []any{}, "total": 1, "current": 1}},
		{"extra_records", map[string]any{"records": []any{managedImportTestRecord()}, "total": 0, "current": 1}},
		{"unbounded", map[string]any{"records": []any{}, "total": 1001, "current": 1}},
		{"too_many_records", map[string]any{"records": make([]importedFileRecord, 101), "total": 101, "current": 1}},
		{"invalid_data", "invalid data"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeManagedImportResponse(w, tc.data)
			}))
			defer server.Close()
			_, err := WithBaseURLAndAPIKey(server.URL, "bound-storage-key").FindImportedURLAtPath(context.Background(), managedImportTestPath, managedImportTestName)
			requireManagedImportError(t, err, "response")
		})
	}
}

func TestManagedURLImportLookupChecksEveryPage(t *testing.T) {
	for _, mode := range []string{"second_page", "ten_pages", "changing_total", "later_conflict", "short_second_page"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				page, _ := strconv.Atoi(r.URL.Query().Get("page"))
				if page != int(calls.Add(1)) || r.Method != http.MethodGet || r.URL.Path != "/api/v1/file/list" {
					t.Error("invalid page traversal or unexpected write")
				}
				total := 101
				if mode == "ten_pages" {
					total = 1000
				}
				size := min(total-(page-1)*100, 100)
				records := make([]importedFileRecord, size)
				if page*100 >= total {
					records[len(records)-1] = managedImportTestRecord()
				}
				if mode == "later_conflict" {
					records[0] = managedImportTestRecord()
					if page == 2 {
						records[0].Size++
					}
				}
				if mode == "changing_total" && page == 2 {
					total++
				}
				if mode == "short_second_page" && page == 2 {
					records = []importedFileRecord{}
				}
				writeManagedImportList(w, records, total, page)
			}))
			defer server.Close()
			got, err := WithBaseURLAndAPIKey(server.URL, "bound-storage-key").FindImportedURLAtPath(context.Background(), managedImportTestPath, managedImportTestName)
			switch mode {
			case "changing_total", "short_second_page":
				requireManagedImportError(t, err, "response")
				if !ClassifyError(err).Retryable {
					t.Fatal("changing/incomplete snapshot should be retryable without a write")
				}
			case "later_conflict":
				requireManagedImportError(t, err, "digest")
			default:
				if err != nil || got.ID != "file-1" {
					t.Fatalf("later page match was not recovered: %v", err)
				}
			}
			wantCalls := int32(2)
			if mode == "ten_pages" {
				wantCalls = 10
			}
			if calls.Load() != wantCalls {
				t.Fatalf("pages=%d want=%d", calls.Load(), wantCalls)
			}
		})
	}
}

func TestManagedURLImportLookupRejectsInvalidMatchedMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		mutate     func(*importedFileRecord)
	}{
		{"missing_hash", "response", func(v *importedFileRecord) { v.HashInfo = nil }},
		{"malformed_hash_string", "hash", func(v *importedFileRecord) { v.HashInfo = json.RawMessage(`"not-json"`) }},
		{"invalid_digest", "response", func(v *importedFileRecord) { v.HashInfo = json.RawMessage(`{"sha256":"bad"}`) }},
		{"no_size", "response", func(v *importedFileRecord) { v.Size = 0 }},
		{"no_locator", "response", func(v *importedFileRecord) { v.URL = "" }},
		{"no_platform", "response", func(v *importedFileRecord) { v.Platform = "" }},
		{"invalid_filename", "response", func(v *importedFileRecord) { v.Filename = "../file.mp4" }},
		{"locator_injection", "locator", func(v *importedFileRecord) { v.URL += "\r\nignored" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				record := managedImportTestRecord()
				tc.mutate(&record)
				writeManagedImportList(w, []importedFileRecord{record}, 1, 1)
			}))
			defer server.Close()
			_, err := WithBaseURLAndAPIKey(server.URL, "bound-storage-key").FindImportedURLAtPath(context.Background(), managedImportTestPath, managedImportTestName)
			requireManagedImportError(t, err, tc.code)
		})
	}
}
