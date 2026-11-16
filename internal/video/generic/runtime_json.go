package generic

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mirainya/Prism/internal/video"
	"github.com/tidwall/gjson"
)

// PrepareMappedSubmit reuses the JSON task mapping for non-video capabilities.
// Only declared fields are sent; caller-provided values never merge into the
// upstream body as arbitrary provider parameters.
func (c *RuntimeCodec) PrepareMappedSubmit(values map[string]any) (RuntimeRequest, error) {
	body := cloneObject(c.adapter.config.Request.FixedBody)
	for source, value := range values {
		target, ok := c.adapter.config.Request.Fields[source]
		if !ok {
			if source == "model" {
				continue
			}
			return RuntimeRequest{}, fmt.Errorf("unsupported input field %s", source)
		}
		if mapping := c.adapter.config.Request.ValueMaps[source]; len(mapping) > 0 {
			mapped, exists := mapping[fmt.Sprint(value)]
			if !exists {
				return RuntimeRequest{}, fmt.Errorf("unsupported input value for %s", source)
			}
			value = mapped
		}
		if err := setField(body, target, value); err != nil {
			return RuntimeRequest{}, err
		}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return RuntimeRequest{}, err
	}
	return c.runtimeRequest(c.adapter.config.Submit, raw, map[string]string{"Content-Type": "application/json"}, ""), nil
}

type TaskResponse struct {
	TaskID string
	Status video.VideoTaskStatus
	URL    string
	Format string
	Error  string
}

// DecodeMappedTask shares the existing root, success, status and task mappings.
// Result selectors and format are supplied by the capability configuration.
func (c *RuntimeCodec) DecodeMappedTask(action string, body []byte, urlPaths, formatPaths []string, defaultFormat string) (TaskResponse, error) {
	response := c.adapter.config.Response
	status := response.SubmitDefaultStatus
	if action == "query" {
		status = response.PollDefaultStatus
	} else if action != "submit" {
		return TaskResponse{}, errors.New("invalid task action")
	}
	if gjson.ValidBytes(body) && response.SuccessCodePath != "" {
		code := gjson.GetBytes(body, response.SuccessCodePath)
		if code.Exists() {
			matched := false
			for _, value := range response.SuccessCodeValues {
				if code.String() == value {
					matched = true
					break
				}
			}
			if !matched {
				return TaskResponse{Status: video.VideoTaskStatusFailed}, nil
			}
		}
	}
	payload, err := c.adapter.responsePayload(body)
	if err != nil {
		return TaskResponse{}, err
	}
	if upstream := strings.ToLower(firstText(payload, response.StatusPaths)); upstream != "" {
		var ok bool
		status, ok = response.StatusMap[upstream]
		if !ok {
			return TaskResponse{}, errors.New("unmapped task status")
		}
	}
	location, err := resolveResultURL(firstText(payload, urlPaths), response.RelativeResultURLBase)
	if err != nil {
		return TaskResponse{}, err
	}
	location, err = appendResultURLQuery(location, response.ResultURLQuery)
	if err != nil {
		return TaskResponse{}, err
	}
	format := firstText(payload, formatPaths)
	if format == "" {
		format = defaultFormat
	}
	out := TaskResponse{TaskID: parseTaskID(firstResult(payload, response.TaskIDPaths)), Status: video.VideoTaskStatus(status),
		URL: location, Format: strings.ToLower(strings.TrimSpace(format)), Error: firstText(payload, response.ErrorPaths)}
	if out.Status == video.VideoTaskStatusCompleted && out.URL == "" {
		return TaskResponse{}, errors.New("completed task is missing result URL")
	}
	return out, nil
}

// MapsInput allows capability validation without exposing the private mapping.
func (c *RuntimeCodec) MapsInput(name string) bool {
	return c.adapter.config.Request.Fields[name] != ""
}
