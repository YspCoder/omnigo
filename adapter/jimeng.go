// Package adapter provides Volcengine Jimeng adaptor implementation.
package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/YspCoder/omnigo/dto"
	"github.com/YspCoder/omnigo/utils"
	"github.com/volcengine/volc-sdk-golang/service/visual"
)

const (
	jimengSubmitAction = "CVSync2AsyncSubmitTask"
	jimengStatusAction = "CVSync2AsyncGetResult"
)

type JimengAdaptor struct {
	client *visual.Visual
}

type jimengSubmitResponse struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
	Data      struct {
		TaskID string `json:"task_id"`
	} `json:"data"`
}

type jimengStatusResponse struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
	Data      struct {
		Status   string `json:"status"`
		VideoURL string `json:"video_url"`
	} `json:"data"`
}

func (a *JimengAdaptor) Chat(context.Context, *ProviderConfig, *dto.MediaRequest) (*dto.MediaResponse, error) {
	return nil, fmt.Errorf("chat mode not supported by Jimeng")
}

func (a *JimengAdaptor) Stream(context.Context, *ProviderConfig, *dto.MediaRequest) (dto.TokenStream, error) {
	return nil, fmt.Errorf("streaming not supported by Jimeng")
}

func (a *JimengAdaptor) Media(ctx context.Context, cfg *ProviderConfig, request *dto.MediaRequest) (*dto.MediaResponse, error) {
	if request == nil || request.Type != dto.MediaTypeVideo {
		return nil, fmt.Errorf("unsupported media mode for Jimeng")
	}
	if request.Duration != 0 && request.Duration != 5 && request.Duration != 10 {
		return nil, fmt.Errorf("Jimeng duration must be 5 or 10 seconds")
	}

	payload := jimengPayload(cfg, request)
	var out jimengSubmitResponse
	if err := a.doJSON(ctx, cfg, jimengSubmitAction, payload, &out); err != nil {
		return nil, err
	}
	if err := jimengResponseError(out.Code, out.Message); err != nil {
		return nil, err
	}
	if strings.TrimSpace(out.Data.TaskID) == "" {
		return nil, fmt.Errorf("Jimeng returned an empty task id")
	}

	return &dto.MediaResponse{
		RequestID: out.RequestID,
		TaskID:    out.Data.TaskID,
		Status:    dto.TaskStatusQueued,
	}, nil
}

func (a *JimengAdaptor) TaskStatus(ctx context.Context, cfg *ProviderConfig, taskID string, query ...map[string]string) (*dto.TaskStatusResponse, error) {
	reqKey := cfg.Model
	payload := map[string]interface{}{
		"req_key": reqKey,
		"task_id": taskID,
	}
	if len(query) > 0 {
		if value := strings.TrimSpace(query[0]["req_key"]); value != "" {
			payload["req_key"] = value
		}
		if value := strings.TrimSpace(query[0]["req_json"]); value != "" {
			payload["req_json"] = value
		}
	}

	var out jimengStatusResponse
	if err := a.doJSON(ctx, cfg, jimengStatusAction, payload, &out); err != nil {
		return nil, err
	}
	if err := jimengResponseError(out.Code, out.Message); err != nil {
		return nil, err
	}
	status := jimengTaskStatus(out.Data.Status)

	return &dto.TaskStatusResponse{
		RequestID: out.RequestID,
		Output: dto.TaskStatusOutput{
			TaskID:     taskID,
			TaskStatus: status,
			URL:        out.Data.VideoURL,
			VideoURL:   out.Data.VideoURL,
		},
	}, nil
}

func (a *JimengAdaptor) ListTasks(context.Context, *ProviderConfig, map[string]string) (*dto.TaskListResponse, error) {
	return nil, fmt.Errorf("task list not supported by Jimeng")
}

func (a *JimengAdaptor) StreamMedia(context.Context, *ProviderConfig, *dto.MediaRequest) (dto.TokenStream, error) {
	return nil, fmt.Errorf("streaming media not supported by Jimeng")
}

func (a *JimengAdaptor) doJSON(ctx context.Context, cfg *ProviderConfig, action string, payload interface{}, out interface{}) error {
	client, err := a.getClient(cfg)
	if err != nil {
		return err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	response, _, err := client.Client.CtxJson(ctx, action, nil, string(body))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(response, out); err != nil {
		return fmt.Errorf("decode Jimeng response: %w", err)
	}
	return nil
}

func (a *JimengAdaptor) getClient(cfg *ProviderConfig) (*visual.Visual, error) {
	if a.client != nil {
		return a.client, nil
	}
	client := visual.NewInstance()
	client.Client.SetAccessKey(cfg.AccessKey)
	client.Client.SetSecretKey(cfg.SecretKey)
	if cfg.Region != "" {
		client.SetRegion(cfg.Region)
	}
	if cfg.Timeout > 0 {
		client.Client.CustomTimeout = cfg.Timeout
	}
	for key, value := range cfg.Headers {
		client.Client.ServiceInfo.Header.Set(key, value)
	}
	if cfg.BaseURL != "" {
		endpoint, err := url.Parse(strings.TrimSpace(cfg.BaseURL))
		if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
			return nil, fmt.Errorf("invalid Jimeng endpoint %q", cfg.BaseURL)
		}
		client.SetHost(endpoint.Host)
		client.SetSchema(endpoint.Scheme)
	}
	if cfg.HTTPClient != nil {
		client.Client.Client = cfg.HTTPClient
	} else if cfg.Proxy != "" {
		proxyURL, err := url.Parse(cfg.Proxy)
		if err != nil {
			return nil, fmt.Errorf("invalid Jimeng proxy: %w", err)
		}
		client.Client.Client = &http.Client{
			Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
			Timeout:   cfg.Timeout,
		}
	}
	a.client = client
	return client, nil
}

func jimengPayload(cfg *ProviderConfig, request *dto.MediaRequest) map[string]interface{} {
	prompt := strings.TrimSpace(utils.MediaPromptWithSystem(request))
	if prompt == "" {
		prompt = strings.TrimSpace(request.Prompt)
	}
	payload := map[string]interface{}{
		"req_key": cfg.Model,
		"prompt":  prompt,
	}
	if request.Duration > 0 {
		payload["frames"] = 24*request.Duration + 1
	}
	if request.Size != "" {
		payload["aspect_ratio"] = request.Size
	}
	if request.Seed != 0 {
		payload["seed"] = request.Seed
	}
	if images := jimengImageURLs(request); len(images) > 0 {
		payload["image_urls"] = images
	}
	for key, value := range request.Extra {
		switch key {
		case "image", "images", "files", "image_url":
			continue
		default:
			payload[key] = value
		}
	}
	return payload
}

func jimengImageURLs(request *dto.MediaRequest) []string {
	images := make([]string, 0, 2)
	for _, message := range request.Messages {
		if imageURL := strings.TrimSpace(message.ImageURL); imageURL != "" {
			images = append(images, imageURL)
		}
		images = append(images, utils.ContentImageURLs(message.Content)...)
	}
	images = append(images, utils.ParseExtraImageInputs(request.Extra)...)
	if imageURL := strings.TrimSpace(utils.GetStringExtra(request.Extra, "image_url")); imageURL != "" {
		images = append(images, imageURL)
	}
	return images
}

func jimengResponseError(code int, message string) error {
	if code == 10000 {
		return nil
	}
	return fmt.Errorf("Jimeng error: code=%d message=%s", code, strings.TrimSpace(message))
}

func jimengTaskStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "in_queue":
		return dto.TaskStatusQueued
	case "generating":
		return dto.TaskStatusInProgress
	case "done":
		return dto.TaskStatusSucceeded
	case "not_found", "expired":
		return dto.TaskStatusFailed
	default:
		normalized, err := dto.NormalizeTaskStatus(status)
		if err != nil {
			return strings.TrimSpace(status)
		}
		return normalized
	}
}
