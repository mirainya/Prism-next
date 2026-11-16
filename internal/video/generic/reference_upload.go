package generic

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/tidwall/gjson"
)

// ReferenceUploadConfig describes a resumable upload contract. Endpoints and
// wire field names belong to the channel configuration, not the executable.
type ReferenceUploadConfig struct {
	Operations     map[string]string         `json:"operations"`
	RequestFields  map[string]string         `json:"request_fields"`
	ResponseFields map[string]string         `json:"response_fields"`
	PartFields     map[string]string         `json:"part_fields"`
	CompletedParts map[string]string         `json:"completed_part_fields"`
	ContentFields  map[string]string         `json:"content_fields"`
	ContentPath    string                    `json:"content_path"`
	FixedBody      map[string]map[string]any `json:"fixed_body,omitempty"`
	SuccessPath    string                    `json:"success_path,omitempty"`
	SuccessValues  []string                  `json:"success_values,omitempty"`
	StateValues    map[string]string         `json:"state_values"`
	ModeValues     map[string]string         `json:"mode_values"`
}

func (c *ReferenceUploadConfig) Validate() error {
	if c == nil {
		return nil
	}
	for _, action := range []string{"prepare", "wait", "complete", "abort"} {
		path := c.Operations[action]
		u, err := url.Parse(path)
		if err != nil || u.IsAbs() || u.Host != "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
			!strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.ContainsAny(path, "\\\r\n") {
			return fmt.Errorf("reference upload %s must use a same-origin absolute path", action)
		}
		for _, segment := range strings.Split(u.Path, "/") {
			if segment == "." || segment == ".." {
				return errors.New("reference upload path contains a relative segment")
			}
		}
	}
	if err := validateJSONFieldPath(c.ContentPath); err != nil {
		return fmt.Errorf("reference upload content_path: %w", err)
	}
	groups := []struct {
		fields map[string]string
		keys   []string
	}{
		{c.RequestFields, []string{"sha256", "size_bytes", "kind", "content_type", "ticket", "session_token", "generation", "parts"}},
		{c.ResponseFields, []string{"disposition", "ticket", "retry_after_seconds", "storage_object_id", "upload.session_token", "upload.generation", "upload.mode", "upload.upload_url", "upload.parts"}},
		{c.PartFields, []string{"part_number", "size_bytes", "upload_url"}},
		{c.CompletedParts, []string{"part_number", "etag"}},
		{c.ContentFields, []string{"url", "type", "reference"}},
	}
	for _, group := range groups {
		if len(group.fields) != len(group.keys) {
			return errors.New("reference upload mapping has missing or unknown fields")
		}
		for _, key := range group.keys {
			if err := validateJSONFieldPath(group.fields[key]); err != nil {
				return fmt.Errorf("reference upload mapping %s: %w", key, err)
			}
		}
	}
	for _, field := range append([]string{c.ContentPath}, mapValues(c.ContentFields, c.CompletedParts)...) {
		if strings.Contains(field, ".") {
			return errors.New("reference upload content and completed part fields must be direct object fields")
		}
	}
	for _, fields := range []map[string]string{c.RequestFields, c.ContentFields, c.CompletedParts} {
		paths := mapValues(fields)
		for i, left := range paths {
			for _, right := range paths[i+1:] {
				if left == right || strings.HasPrefix(left, right+".") || strings.HasPrefix(right, left+".") {
					return errors.New("reference upload field mappings overlap")
				}
			}
		}
	}
	if (c.SuccessPath == "") != (len(c.SuccessValues) == 0) {
		return errors.New("reference upload success path and values must be configured together")
	}
	if c.SuccessPath != "" {
		if err := validateJSONFieldPath(c.SuccessPath); err != nil {
			return err
		}
	}
	for _, group := range []struct {
		values map[string]string
		keys   []string
	}{
		{c.StateValues, []string{"owner", "waiting", "reused"}},
		{c.ModeValues, []string{"single", "multipart"}},
	} {
		if len(group.values) != len(group.keys) {
			return errors.New("reference upload has missing or unknown state values")
		}
		seen := map[string]bool{}
		for _, key := range group.keys {
			value := group.values[key]
			if value == "" || seen[value] {
				return errors.New("reference upload state and mode values must be nonempty and distinct")
			}
			seen[value] = true
		}
	}
	return nil
}

func (c *ReferenceUploadConfig) RequestBody(action string, values map[string]any) ([]byte, error) {
	body := cloneObject(c.FixedBody[action])
	for key, value := range values {
		path := c.RequestFields[key]
		if path == "" {
			return nil, fmt.Errorf("reference upload input %s is not mapped", key)
		}
		if err := setField(body, path, value); err != nil {
			return nil, err
		}
	}
	return json.Marshal(body)
}

// DecodeResponse normalizes mapped fields into the upload executor's contract.
// A missing success marker fails closed when a check has been configured.
func (c *ReferenceUploadConfig) DecodeResponse(body []byte) ([]byte, error) {
	if !gjson.ValidBytes(body) {
		return nil, errors.New("invalid reference upload JSON")
	}
	if c.SuccessPath != "" {
		value := gjson.GetBytes(body, c.SuccessPath)
		matched := false
		for _, allowed := range c.SuccessValues {
			matched = matched || (value.Exists() && value.String() == allowed)
		}
		if !matched {
			return nil, errors.New("reference upload success check failed")
		}
	}
	out := map[string]any{}
	for key, path := range c.ResponseFields {
		value := gjson.GetBytes(body, path)
		if !value.Exists() {
			continue
		}
		var normalized any = json.RawMessage(value.Raw)
		if key == "disposition" || key == "upload.mode" {
			values := c.StateValues
			if key == "upload.mode" {
				values = c.ModeValues
			}
			matched := false
			for canonical, wire := range values {
				if value.String() == wire {
					normalized = canonical
					matched = true
					break
				}
			}
			if !matched {
				return nil, errors.New("unmapped reference upload state or mode")
			}
		}
		if key == "upload.parts" {
			if !value.IsArray() {
				return nil, errors.New("reference upload parts must be an array")
			}
			parts := []map[string]any{}
			for _, part := range value.Array() {
				mapped := map[string]any{}
				for name, selector := range c.PartFields {
					field := part.Get(selector)
					if field.Exists() {
						mapped[name] = json.RawMessage(field.Raw)
					}
				}
				parts = append(parts, mapped)
			}
			normalized = parts
		}
		if err := setField(out, key, normalized); err != nil {
			return nil, err
		}
	}
	return json.Marshal(out)
}

func mapValues(groups ...map[string]string) []string {
	var out []string
	for _, group := range groups {
		for _, value := range group {
			out = append(out, value)
		}
	}
	return out
}
