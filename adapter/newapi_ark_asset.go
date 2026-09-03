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
	"strings"
)

const (
	newAPIArkAssetAPIVersion = "2024-01-01"

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
	Name      string `json:"Name"`
	URL       string `json:"URL"`
	AssetType string `json:"AssetType"`
	GroupID   string `json:"GroupId"`
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
	return newAPIArkAssetCall[NewAPIArkResourceID](ctx, cfg, "CreateAsset", request)
}

func (a *NewAPIArkAssetAdaptor) ListAssets(ctx context.Context, cfg *ProviderConfig, request *NewAPIArkListAssetsRequest) (*NewAPIArkAssetList, error) {
	if request == nil {
		return nil, fmt.Errorf("list NewAPI Ark assets request is required")
	}
	return newAPIArkAssetCall[NewAPIArkAssetList](ctx, cfg, "ListAssets", request)
}

func (a *NewAPIArkAssetAdaptor) GetAsset(ctx context.Context, cfg *ProviderConfig, request *NewAPIArkGetAssetRequest) (*NewAPIArkAsset, error) {
	if request == nil {
		return nil, fmt.Errorf("get NewAPI Ark asset request is required")
	}
	return newAPIArkAssetCall[NewAPIArkAsset](ctx, cfg, "GetAsset", request)
}

func (a *NewAPIArkAssetAdaptor) UpdateAsset(ctx context.Context, cfg *ProviderConfig, request *NewAPIArkUpdateAssetRequest) (*NewAPIArkResourceID, error) {
	if request == nil {
		return nil, fmt.Errorf("update NewAPI Ark asset request is required")
	}
	return newAPIArkAssetCall[NewAPIArkResourceID](ctx, cfg, "UpdateAsset", request)
}

func (a *NewAPIArkAssetAdaptor) DeleteAsset(ctx context.Context, cfg *ProviderConfig, request *NewAPIArkDeleteAssetRequest) (*NewAPIArkDeleteResult, error) {
	if request == nil {
		return nil, fmt.Errorf("delete NewAPI Ark asset request is required")
	}
	return newAPIArkAssetCall[NewAPIArkDeleteResult](ctx, cfg, "DeleteAsset", request)
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
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/"
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
