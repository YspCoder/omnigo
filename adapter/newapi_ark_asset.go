// Package adapter provides the NewAPI Ark Action asset adaptor.
package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	newAPIArkAssetAPIVersion = "2024-01-01"
	newAPIArkAssetAPIPath    = "/v1/ark/asset"

	NewAPIArkAssetTypeImage = "Image"

	NewAPIArkAssetStatusProcessing = "Processing"
	NewAPIArkAssetStatusActive     = "Active"
	NewAPIArkAssetStatusFailed     = "Failed"
)

// NewAPIArkAssetAdaptor implements the NewAPI Ark Action asset protocol.
type NewAPIArkAssetAdaptor struct{}

// NewAPIArkResponseMetadata is returned with every NewAPI Ark asset response.
type NewAPIArkResponseMetadata struct {
	RequestID string                `json:"RequestId"`
	Action    string                `json:"Action"`
	Version   string                `json:"Version"`
	Service   string                `json:"Service"`
	Region    string                `json:"Region"`
	Error     *NewAPIArkErrorDetail `json:"Error,omitempty"`
}

// NewAPIArkErrorDetail describes an error returned in ResponseMetadata.
type NewAPIArkErrorDetail struct {
	Code    string `json:"Code"`
	Message string `json:"Message"`
}

// NewAPIArkAssetAPIError is returned for HTTP and business errors from NewAPI Ark.
type NewAPIArkAssetAPIError struct {
	StatusCode int
	RequestID  string
	Action     string
	Code       string
	Message    string
}

func (e *NewAPIArkAssetAPIError) Error() string {
	parts := []string{"NewAPI Ark asset API error"}
	if e.StatusCode != 0 {
		parts = append(parts, fmt.Sprintf("status=%d", e.StatusCode))
	}
	if e.Action != "" {
		parts = append(parts, "action="+e.Action)
	}
	if e.Code != "" {
		parts = append(parts, "code="+e.Code)
	}
	if e.RequestID != "" {
		parts = append(parts, "request_id="+e.RequestID)
	}
	if e.Message != "" {
		parts = append(parts, "message="+e.Message)
	}
	return strings.Join(parts, " ")
}

// NewAPIArkResourceID is returned by create and update actions.
type NewAPIArkResourceID struct {
	ID               string                    `json:"Id"`
	ResponseMetadata NewAPIArkResponseMetadata `json:"-"`
}

// NewAPIArkDeleteResult represents the empty Result object returned by delete actions.
type NewAPIArkDeleteResult struct {
	ResponseMetadata NewAPIArkResponseMetadata `json:"-"`
}

// NewAPIArkAssetGroup is a NewAPI Ark AIGC asset group.
type NewAPIArkAssetGroup struct {
	ID               string                    `json:"Id"`
	Name             string                    `json:"Name"`
	Description      string                    `json:"Description"`
	GroupType        string                    `json:"GroupType"`
	ProjectName      string                    `json:"ProjectName"`
	CreateTime       string                    `json:"CreateTime"`
	UpdateTime       string                    `json:"UpdateTime"`
	ResponseMetadata NewAPIArkResponseMetadata `json:"-"`
}

type NewAPIArkCreateAssetGroupRequest struct {
	Name        string `json:"Name"`
	Description string `json:"Description,omitempty"`
}

type NewAPIArkAssetGroupFilter struct {
	GroupIDs  []string `json:"GroupIds,omitempty"`
	Name      string   `json:"Name,omitempty"`
	GroupType string   `json:"GroupType,omitempty"`
}

type NewAPIArkListAssetGroupsRequest struct {
	Filter     *NewAPIArkAssetGroupFilter `json:"Filter,omitempty"`
	PageNumber int                        `json:"PageNumber,omitempty"`
	PageSize   int                        `json:"PageSize,omitempty"`
	SortBy     string                     `json:"SortBy,omitempty"`
	SortOrder  string                     `json:"SortOrder,omitempty"`
}

type NewAPIArkAssetGroupList struct {
	TotalCount       int                       `json:"TotalCount"`
	Items            []NewAPIArkAssetGroup     `json:"Items"`
	PageNumber       int                       `json:"PageNumber"`
	PageSize         int                       `json:"PageSize"`
	ResponseMetadata NewAPIArkResponseMetadata `json:"-"`
}

type NewAPIArkGetAssetGroupRequest struct {
	ID          string `json:"Id"`
	ProjectName string `json:"ProjectName,omitempty"`
}

type NewAPIArkUpdateAssetGroupRequest struct {
	ID          string  `json:"Id"`
	Name        *string `json:"Name,omitempty"`
	Description *string `json:"Description,omitempty"`
}

type NewAPIArkDeleteAssetGroupRequest struct {
	ID          string `json:"Id"`
	ProjectName string `json:"ProjectName,omitempty"`
}

// NewAPIArkAsset is an asset registered in the NewAPI Ark resource library.
type NewAPIArkAsset struct {
	ID               string                    `json:"Id"`
	Name             string                    `json:"Name"`
	URL              string                    `json:"URL"`
	AssetType        string                    `json:"AssetType"`
	GroupID          string                    `json:"GroupId"`
	Status           string                    `json:"Status"`
	Moderation       NewAPIArkModeration       `json:"Moderation"`
	Error            any                       `json:"Error,omitempty"`
	CreateTime       string                    `json:"CreateTime"`
	UpdateTime       string                    `json:"UpdateTime"`
	ProjectName      string                    `json:"ProjectName"`
	ResponseMetadata NewAPIArkResponseMetadata `json:"-"`
}

type NewAPIArkModeration struct {
	Strategy string `json:"Strategy"`
}

type NewAPIArkCreateAssetRequest struct {
	Purpose   string `json:"purpose"`
	URL       string `json:"url"`
	AssetType string `json:"type,omitempty"`
	GroupID   string `json:"group_id,omitempty"`
}

type NewAPIArkAssetFilter struct {
	GroupIDs  []string `json:"GroupIds,omitempty"`
	GroupType string   `json:"GroupType,omitempty"`
	Name      string   `json:"Name,omitempty"`
}

type NewAPIArkListAssetsRequest struct {
	Filter     *NewAPIArkAssetFilter `json:"Filter,omitempty"`
	PageNumber int                   `json:"PageNumber,omitempty"`
	PageSize   int                   `json:"PageSize,omitempty"`
	SortBy     string                `json:"SortBy,omitempty"`
	SortOrder  string                `json:"SortOrder,omitempty"`
}

type NewAPIArkAssetList struct {
	Items            []NewAPIArkAsset          `json:"Items"`
	TotalCount       int                       `json:"TotalCount"`
	PageNumber       int                       `json:"PageNumber"`
	PageSize         int                       `json:"PageSize"`
	ResponseMetadata NewAPIArkResponseMetadata `json:"-"`
}

type NewAPIArkGetAssetRequest struct {
	ID string `json:"Id"`
}

type NewAPIArkUpdateAssetRequest struct {
	ID   string `json:"Id"`
	Name string `json:"Name"`
}

type NewAPIArkDeleteAssetRequest struct {
	ID string `json:"Id"`
}

func (a *NewAPIArkAssetAdaptor) CreateAssetGroup(ctx context.Context, cfg *ProviderConfig, request *NewAPIArkCreateAssetGroupRequest) (*NewAPIArkResourceID, error) {
	if request == nil {
		return nil, fmt.Errorf("create NewAPI Ark asset group request is required")
	}
	return newAPIArkAssetCall[NewAPIArkResourceID](ctx, cfg, "CreateAssetGroup", request)
}

func (a *NewAPIArkAssetAdaptor) ListAssetGroups(ctx context.Context, cfg *ProviderConfig, request *NewAPIArkListAssetGroupsRequest) (*NewAPIArkAssetGroupList, error) {
	if request == nil {
		return nil, fmt.Errorf("list NewAPI Ark asset groups request is required")
	}
	return newAPIArkAssetCall[NewAPIArkAssetGroupList](ctx, cfg, "ListAssetGroups", request)
}

func (a *NewAPIArkAssetAdaptor) GetAssetGroup(ctx context.Context, cfg *ProviderConfig, request *NewAPIArkGetAssetGroupRequest) (*NewAPIArkAssetGroup, error) {
	if request == nil {
		return nil, fmt.Errorf("get NewAPI Ark asset group request is required")
	}
	return newAPIArkAssetCall[NewAPIArkAssetGroup](ctx, cfg, "GetAssetGroup", request)
}

func (a *NewAPIArkAssetAdaptor) UpdateAssetGroup(ctx context.Context, cfg *ProviderConfig, request *NewAPIArkUpdateAssetGroupRequest) (*NewAPIArkResourceID, error) {
	if request == nil {
		return nil, fmt.Errorf("update NewAPI Ark asset group request is required")
	}
	return newAPIArkAssetCall[NewAPIArkResourceID](ctx, cfg, "UpdateAssetGroup", request)
}

func (a *NewAPIArkAssetAdaptor) DeleteAssetGroup(ctx context.Context, cfg *ProviderConfig, request *NewAPIArkDeleteAssetGroupRequest) (*NewAPIArkDeleteResult, error) {
	if request == nil {
		return nil, fmt.Errorf("delete NewAPI Ark asset group request is required")
	}
	return newAPIArkAssetCall[NewAPIArkDeleteResult](ctx, cfg, "DeleteAssetGroup", request)
}

func (a *NewAPIArkAssetAdaptor) CreateAsset(ctx context.Context, cfg *ProviderConfig, request *NewAPIArkCreateAssetRequest) (*NewAPIArkResourceID, error) {
	if request == nil {
		return nil, fmt.Errorf("create NewAPI Ark asset request is required")
	}
	payload := *request
	payload.Purpose = strings.TrimSpace(payload.Purpose)
	if payload.Purpose == "" {
		return nil, fmt.Errorf("create NewAPI Ark asset purpose is required")
	}
	payload.AssetType = strings.ToLower(strings.TrimSpace(payload.AssetType))
	result, err := newAPIArkRESTCall[newAPIArkRESTAsset](ctx, cfg, http.MethodPost, "/v1/assets/upload", nil, &payload)
	if err != nil {
		return nil, err
	}
	return &NewAPIArkResourceID{ID: result.AssetID}, nil
}

func (a *NewAPIArkAssetAdaptor) ListAssets(ctx context.Context, cfg *ProviderConfig, request *NewAPIArkListAssetsRequest) (*NewAPIArkAssetList, error) {
	if request == nil {
		return nil, fmt.Errorf("list NewAPI Ark assets request is required")
	}
	query := make(url.Values)
	if request.PageNumber > 0 {
		query.Set("page", strconv.Itoa(request.PageNumber))
	}
	if request.PageSize > 0 {
		query.Set("page_size", strconv.Itoa(request.PageSize))
	}
	if request.SortOrder != "" {
		query.Set("sort_order", request.SortOrder)
	}
	if request.Filter != nil {
		if request.Filter.Name != "" {
			query.Set("purpose", request.Filter.Name)
		}
		if len(request.Filter.GroupIDs) > 0 {
			query.Set("group_id", request.Filter.GroupIDs[0])
		}
	}
	result, err := newAPIArkRESTCall[newAPIArkRESTAssetList](ctx, cfg, http.MethodGet, "/v1/assets", query, nil)
	if err != nil {
		return nil, err
	}
	items := make([]NewAPIArkAsset, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, item.toNewAPIArkAsset())
	}
	return &NewAPIArkAssetList{
		Items:      items,
		TotalCount: result.Total,
		PageNumber: result.Page,
		PageSize:   result.PageSize,
	}, nil
}

func (a *NewAPIArkAssetAdaptor) GetAsset(ctx context.Context, cfg *ProviderConfig, request *NewAPIArkGetAssetRequest) (*NewAPIArkAsset, error) {
	if request == nil {
		return nil, fmt.Errorf("get NewAPI Ark asset request is required")
	}
	result, err := newAPIArkRESTCall[newAPIArkRESTAsset](ctx, cfg, http.MethodGet, "/v1/assets/"+request.ID, nil, nil)
	if err != nil {
		return nil, err
	}
	asset := result.toNewAPIArkAsset()
	return &asset, nil
}

func (a *NewAPIArkAssetAdaptor) UpdateAsset(ctx context.Context, cfg *ProviderConfig, request *NewAPIArkUpdateAssetRequest) (*NewAPIArkResourceID, error) {
	if request == nil {
		return nil, fmt.Errorf("update NewAPI Ark asset request is required")
	}
	payload := struct {
		Purpose string `json:"purpose"`
	}{Purpose: request.Name}
	result, err := newAPIArkRESTCall[newAPIArkRESTAsset](ctx, cfg, http.MethodPut, "/v1/assets/"+request.ID, nil, &payload)
	if err != nil {
		return nil, err
	}
	return &NewAPIArkResourceID{ID: result.AssetID}, nil
}

func (a *NewAPIArkAssetAdaptor) DeleteAsset(ctx context.Context, cfg *ProviderConfig, request *NewAPIArkDeleteAssetRequest) (*NewAPIArkDeleteResult, error) {
	if request == nil {
		return nil, fmt.Errorf("delete NewAPI Ark asset request is required")
	}
	_, err := newAPIArkRESTCall[newAPIArkRESTAsset](ctx, cfg, http.MethodDelete, "/v1/assets/"+request.ID, nil, nil)
	if err != nil {
		return nil, err
	}
	return &NewAPIArkDeleteResult{}, nil
}

type newAPIArkRESTEnvelope[T any] struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type newAPIArkRESTAsset struct {
	AssetID       string `json:"asset_id"`
	GroupID       string `json:"group_id"`
	OriginalURL   string `json:"original_url"`
	Purpose       string `json:"purpose"`
	Status        string `json:"status"`
	Type          string `json:"type"`
	FailureReason string `json:"failure_reason,omitempty"`
}

func (a newAPIArkRESTAsset) toNewAPIArkAsset() NewAPIArkAsset {
	status := a.Status
	switch strings.ToLower(status) {
	case "pending", "processing":
		status = NewAPIArkAssetStatusProcessing
	case "ready", "active":
		status = NewAPIArkAssetStatusActive
	case "failed":
		status = NewAPIArkAssetStatusFailed
	}
	return NewAPIArkAsset{
		ID:        a.AssetID,
		Name:      a.Purpose,
		URL:       a.OriginalURL,
		AssetType: a.Type,
		GroupID:   a.GroupID,
		Status:    status,
		Error:     a.FailureReason,
	}
}

type newAPIArkRESTAssetList struct {
	Total    int                  `json:"total"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"page_size"`
	Items    []newAPIArkRESTAsset `json:"items"`
}

type newAPIArkAssetEnvelope struct {
	ResponseMetadata NewAPIArkResponseMetadata `json:"ResponseMetadata"`
	Result           json.RawMessage           `json:"Result"`
}

func newAPIArkAssetCall[T any](ctx context.Context, cfg *ProviderConfig, action string, payload any) (*T, error) {
	endpoint, err := newAPIArkAssetEndpoint(cfg, action)
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode NewAPI Ark asset request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create NewAPI Ark asset request: %w", err)
	}
	if cfg != nil {
		setNewAPIArkAssetHeaders(request, cfg.Headers)
	}
	request.Header.Set("Content-Type", "application/json")
	if err := newAPIArkAssetAuthenticate(request, cfg); err != nil {
		return nil, err
	}

	response, err := newAPIArkAssetHTTPClient(cfg).Do(request)
	if err != nil {
		return nil, fmt.Errorf("call NewAPI Ark asset API: %w", err)
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read NewAPI Ark asset API response: %w", err)
	}

	var envelope newAPIArkAssetEnvelope
	decodeErr := json.Unmarshal(raw, &envelope)
	if decodeErr == nil && envelope.ResponseMetadata.Error != nil {
		return nil, newAPIArkAssetError(response.StatusCode, action, envelope.ResponseMetadata, envelope.ResponseMetadata.Error.Message)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message := strings.TrimSpace(string(raw))
		if decodeErr != nil {
			message = fmt.Sprintf("%s (decode response: %v)", message, decodeErr)
		}
		return nil, newAPIArkAssetError(response.StatusCode, action, envelope.ResponseMetadata, message)
	}
	if decodeErr != nil {
		return nil, fmt.Errorf("decode NewAPI Ark asset API response: %w", decodeErr)
	}

	var result T
	if len(envelope.Result) != 0 && string(envelope.Result) != "null" {
		if err := json.Unmarshal(envelope.Result, &result); err != nil {
			return nil, fmt.Errorf("decode NewAPI Ark asset API result: %w", err)
		}
	}
	setNewAPIArkResponseMetadata(&result, envelope.ResponseMetadata)
	return &result, nil
}

func newAPIArkRESTCall[T any](ctx context.Context, cfg *ProviderConfig, method, path string, query url.Values, payload any) (*T, error) {
	endpoint, err := newAPIArkRESTEndpoint(cfg, path, query)
	if err != nil {
		return nil, err
	}

	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("encode NewAPI Ark asset request: %w", err)
		}
		body = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("create NewAPI Ark asset request: %w", err)
	}
	if cfg != nil {
		setNewAPIArkAssetHeaders(request, cfg.Headers)
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if err := newAPIArkAssetAuthenticate(request, cfg); err != nil {
		return nil, err
	}

	response, err := newAPIArkAssetHTTPClient(cfg).Do(request)
	if err != nil {
		return nil, fmt.Errorf("call NewAPI Ark asset API: %w", err)
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read NewAPI Ark asset API response: %w", err)
	}
	var envelope newAPIArkRESTEnvelope[T]
	decodeErr := json.Unmarshal(raw, &envelope)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message := strings.TrimSpace(string(raw))
		if decodeErr == nil && strings.TrimSpace(envelope.Message) != "" {
			message = envelope.Message
		}
		return nil, newAPIArkAssetError(response.StatusCode, "", NewAPIArkResponseMetadata{}, message)
	}
	if decodeErr != nil {
		return nil, fmt.Errorf("decode NewAPI Ark asset API response: %w", decodeErr)
	}
	if !envelope.Success {
		return nil, newAPIArkAssetError(response.StatusCode, "", NewAPIArkResponseMetadata{}, envelope.Message)
	}
	return &envelope.Data, nil
}

func newAPIArkRESTEndpoint(cfg *ProviderConfig, path string, query url.Values) (string, error) {
	if cfg == nil || strings.TrimSpace(cfg.BaseURL) == "" {
		return "", fmt.Errorf("NewAPI Ark asset API base URL is required")
	}
	rawBaseURL := strings.TrimSpace(cfg.BaseURL)
	endpoint, err := url.Parse(rawBaseURL)
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return "", fmt.Errorf("NewAPI Ark asset API base URL must be an absolute http(s) URL: %q", rawBaseURL)
	}
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/" + strings.TrimLeft(path, "/")
	endpoint.RawPath = ""
	if query == nil {
		endpoint.RawQuery = ""
	} else {
		endpoint.RawQuery = query.Encode()
	}
	endpoint.Fragment = ""
	return endpoint.String(), nil
}

func setNewAPIArkResponseMetadata(result any, metadata NewAPIArkResponseMetadata) {
	switch value := result.(type) {
	case *NewAPIArkResourceID:
		value.ResponseMetadata = metadata
	case *NewAPIArkDeleteResult:
		value.ResponseMetadata = metadata
	case *NewAPIArkAssetGroup:
		value.ResponseMetadata = metadata
	case *NewAPIArkAssetGroupList:
		value.ResponseMetadata = metadata
	case *NewAPIArkAsset:
		value.ResponseMetadata = metadata
	case *NewAPIArkAssetList:
		value.ResponseMetadata = metadata
	}
}

func newAPIArkAssetEndpoint(cfg *ProviderConfig, action string) (string, error) {
	if cfg == nil || strings.TrimSpace(cfg.BaseURL) == "" {
		return "", fmt.Errorf("NewAPI Ark asset API base URL is required")
	}
	rawBaseURL := strings.TrimSpace(cfg.BaseURL)
	endpoint, err := url.Parse(rawBaseURL)
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return "", fmt.Errorf("NewAPI Ark asset API base URL must be an absolute http(s) URL: %q", rawBaseURL)
	}
	basePath := strings.TrimRight(endpoint.Path, "/")
	if !strings.HasSuffix(basePath, newAPIArkAssetAPIPath) {
		basePath += newAPIArkAssetAPIPath
	}
	endpoint.Path = basePath
	endpoint.RawPath = ""
	query := endpoint.Query()
	query.Set("Action", action)
	query.Set("Version", newAPIArkAssetAPIVersion)
	endpoint.RawQuery = query.Encode()
	endpoint.Fragment = ""
	return endpoint.String(), nil
}

func newAPIArkAssetAuthenticate(request *http.Request, cfg *ProviderConfig) error {
	if cfg == nil || strings.TrimSpace(cfg.APIKey) == "" {
		return fmt.Errorf("NewAPI Ark asset API key is required")
	}
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.APIKey))
	return nil
}

func setNewAPIArkAssetHeaders(request *http.Request, headers map[string]string) {
	for key, value := range headers {
		request.Header.Set(key, value)
	}
}

func newAPIArkAssetHTTPClient(cfg *ProviderConfig) *http.Client {
	if cfg != nil && cfg.HTTPClient != nil {
		return cfg.HTTPClient
	}
	client := &http.Client{}
	if cfg != nil && cfg.Timeout > 0 {
		client.Timeout = cfg.Timeout
	}
	return client
}

func newAPIArkAssetError(statusCode int, action string, metadata NewAPIArkResponseMetadata, fallback string) *NewAPIArkAssetAPIError {
	err := &NewAPIArkAssetAPIError{
		StatusCode: statusCode,
		RequestID:  metadata.RequestID,
		Action:     action,
		Message:    fallback,
	}
	if metadata.Action != "" {
		err.Action = metadata.Action
	}
	if metadata.Error != nil {
		err.Code = metadata.Error.Code
		err.Message = metadata.Error.Message
	}
	return err
}
