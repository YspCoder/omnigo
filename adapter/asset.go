// Package adapter provides asset management adaptors.
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

// AssetAdaptor defines resource group and asset management operations.
type AssetAdaptor interface {
	CreateAssetGroup(context.Context, *ProviderConfig, *CreateAssetGroupRequest) (*AssetGroup, error)
	ListAssetGroups(context.Context, *ProviderConfig, *ListAssetGroupsRequest) (*AssetGroupList, error)
	GetAssetGroup(context.Context, *ProviderConfig, int64) (*AssetGroup, error)
	UpdateAssetGroup(context.Context, *ProviderConfig, int64, *UpdateAssetGroupRequest) (*AssetGroup, error)
	DeleteAssetGroup(context.Context, *ProviderConfig, int64) (*DeleteAssetResult, error)

	CreateAsset(context.Context, *ProviderConfig, *CreateAssetRequest) (*Asset, error)
	ListAssets(context.Context, *ProviderConfig, *ListAssetsRequest) (*AssetList, error)
	GetAsset(context.Context, *ProviderConfig, int64) (*Asset, error)
	UpdateAsset(context.Context, *ProviderConfig, int64, *UpdateAssetRequest) (*Asset, error)
	DeleteAsset(context.Context, *ProviderConfig, int64) (*DeleteAssetResult, error)
}

// NewAPIAssetAdaptor implements asset management against the newapi endpoints.
type NewAPIAssetAdaptor struct{}

var _ AssetAdaptor = (*NewAPIAssetAdaptor)(nil)

// AssetGroup is a logical container for managed assets.
type AssetGroup struct {
	ID               int64                      `json:"id"`
	UserID           int64                      `json:"user_id,omitempty"`
	CloudGroupID     string                     `json:"cloud_group_id"`
	GroupType        string                     `json:"group_type"`
	GroupName        string                     `json:"group_name"`
	Description      string                     `json:"description"`
	CreatedAt        int64                      `json:"created_at"`
	UpdatedAt        int64                      `json:"updated_at"`
	CreateTime       string                     `json:"create_time,omitempty"`
	UpdateTime       string                     `json:"update_time,omitempty"`
	ProjectName      string                     `json:"project_name,omitempty"`
	ResponseMetadata *NewAPIArkResponseMetadata `json:"-"`
}

// CreateAssetGroupRequest contains the fields accepted when creating an asset group.
type CreateAssetGroupRequest struct {
	GroupType   string `json:"group_type"`
	GroupName   string `json:"group_name"`
	Description string `json:"description,omitempty"`
}

// ListAssetGroupsRequest contains asset group pagination and filtering fields.
type ListAssetGroupsRequest struct {
	PageNo    int      `json:"page_no"`
	PageSize  int      `json:"page_size"`
	GroupType string   `json:"group_type,omitempty"`
	GroupIDs  []string `json:"group_ids,omitempty"`
	GroupName string   `json:"group_name,omitempty"`
	SortBy    string   `json:"sort_by,omitempty"`
	SortOrder string   `json:"sort_order,omitempty"`
}

// AssetGroupList is a page of asset groups.
type AssetGroupList struct {
	Items            []AssetGroup               `json:"items"`
	Total            int                        `json:"total"`
	PageNo           int                        `json:"page_no"`
	PageSize         int                        `json:"page_size"`
	ResponseMetadata *NewAPIArkResponseMetadata `json:"-"`
}

// UpdateAssetGroupRequest contains optional asset group fields to update.
type UpdateAssetGroupRequest struct {
	GroupName   *string `json:"group_name,omitempty"`
	Description *string `json:"description,omitempty"`
}

// Asset is a managed image, video, or audio file.
type Asset struct {
	ID               int64                      `json:"id"`
	CloudAssetID     string                     `json:"cloud_asset_id"`
	CloudGroupID     string                     `json:"cloud_group_id"`
	AssetType        string                     `json:"asset_type"`
	AssetStatus      string                     `json:"asset_status"`
	AssetName        string                     `json:"asset_name"`
	AssetURL         string                     `json:"asset_url"`
	CreatedAt        int64                      `json:"created_at"`
	UpdatedAt        int64                      `json:"updated_at"`
	CreateTime       string                     `json:"create_time,omitempty"`
	UpdateTime       string                     `json:"update_time,omitempty"`
	ProjectName      string                     `json:"project_name,omitempty"`
	Moderation       any                        `json:"moderation,omitempty"`
	Error            any                        `json:"error,omitempty"`
	ResponseMetadata *NewAPIArkResponseMetadata `json:"-"`
}

// CreateAssetRequest contains the fields accepted when creating an asset.
type CreateAssetRequest struct {
	CloudGroupID string `json:"cloud_group_id"`
	AssetName    string `json:"asset_name"`
	AssetURL     string `json:"asset_url"`
	AssetType    string `json:"asset_type"`
	Purpose      string `json:"purpose,omitempty"`
}

// ListAssetsRequest contains asset pagination fields for one asset group.
type ListAssetsRequest struct {
	CloudGroupID string   `json:"cloud_group_id,omitempty"`
	GroupIDs     []string `json:"group_ids,omitempty"`
	GroupType    string   `json:"group_type,omitempty"`
	AssetName    string   `json:"asset_name,omitempty"`
	PageNo       int      `json:"page_no"`
	PageSize     int      `json:"page_size"`
	SortBy       string   `json:"sort_by,omitempty"`
	SortOrder    string   `json:"sort_order,omitempty"`
}

// AssetList is a page of assets.
type AssetList struct {
	Items            []Asset                    `json:"items"`
	Total            int                        `json:"total"`
	PageNo           int                        `json:"page_no"`
	PageSize         int                        `json:"page_size"`
	ResponseMetadata *NewAPIArkResponseMetadata `json:"-"`
}

// UpdateAssetRequest contains the new name for an asset.
type UpdateAssetRequest struct {
	AssetName string `json:"asset_name"`
}

// DeleteAssetResult reports whether an asset or asset group was deleted.
type DeleteAssetResult struct {
	Deleted          bool                       `json:"deleted"`
	ResponseMetadata *NewAPIArkResponseMetadata `json:"-"`
}

// CreateAssetGroup creates an asset group.
func (a *NewAPIAssetAdaptor) CreateAssetGroup(ctx context.Context, cfg *ProviderConfig, request *CreateAssetGroupRequest) (*AssetGroup, error) {
	if request == nil {
		return nil, fmt.Errorf("create asset group request is required")
	}
	return newAPIAssetCall[AssetGroup](ctx, a, cfg, http.MethodPost, "/v1/assets/groups", request)
}

// ListAssetGroups returns a page of asset groups.
func (a *NewAPIAssetAdaptor) ListAssetGroups(ctx context.Context, cfg *ProviderConfig, request *ListAssetGroupsRequest) (*AssetGroupList, error) {
	if request == nil {
		return nil, fmt.Errorf("list asset groups request is required")
	}
	return newAPIAssetCall[AssetGroupList](ctx, a, cfg, http.MethodPost, "/v1/assets/groups/list", request)
}

// GetAssetGroup returns an asset group by its local ID.
func (a *NewAPIAssetAdaptor) GetAssetGroup(ctx context.Context, cfg *ProviderConfig, id int64) (*AssetGroup, error) {
	return newAPIAssetCall[AssetGroup](ctx, a, cfg, http.MethodGet, fmt.Sprintf("/v1/assets/groups/%d", id), nil)
}

// UpdateAssetGroup updates an asset group by its local ID.
func (a *NewAPIAssetAdaptor) UpdateAssetGroup(ctx context.Context, cfg *ProviderConfig, id int64, request *UpdateAssetGroupRequest) (*AssetGroup, error) {
	if request == nil {
		return nil, fmt.Errorf("update asset group request is required")
	}
	return newAPIAssetCall[AssetGroup](ctx, a, cfg, http.MethodPut, fmt.Sprintf("/v1/assets/groups/%d", id), request)
}

// DeleteAssetGroup deletes an asset group and its assets by the group's local ID.
func (a *NewAPIAssetAdaptor) DeleteAssetGroup(ctx context.Context, cfg *ProviderConfig, id int64) (*DeleteAssetResult, error) {
	return newAPIAssetCall[DeleteAssetResult](ctx, a, cfg, http.MethodDelete, fmt.Sprintf("/v1/assets/groups/%d", id), nil)
}

// CreateAsset creates an asset in an asset group.
func (a *NewAPIAssetAdaptor) CreateAsset(ctx context.Context, cfg *ProviderConfig, request *CreateAssetRequest) (*Asset, error) {
	if request == nil {
		return nil, fmt.Errorf("create asset request is required")
	}
	purpose := strings.TrimSpace(request.Purpose)
	if purpose == "" {
		purpose = request.AssetName
	}
	payload := &newAPIUploadAssetRequest{
		URL:     request.AssetURL,
		Purpose: purpose,
		Type:    strings.ToLower(strings.TrimSpace(request.AssetType)),
		GroupID: request.CloudGroupID,
	}
	result, err := newAPIAssetCall[newAPIUploadAssetResult](ctx, a, cfg, http.MethodPost, "/v1/assets/upload", payload)
	if err != nil {
		return nil, err
	}
	return &Asset{
		CloudAssetID: result.AssetID,
		CloudGroupID: result.GroupID,
		AssetType:    payload.Type,
		AssetStatus:  result.Status,
		AssetName:    request.AssetName,
		AssetURL:     request.AssetURL,
		Error:        result.FailureReason,
	}, nil
}

type newAPIUploadAssetRequest struct {
	URL     string `json:"url"`
	Purpose string `json:"purpose"`
	Type    string `json:"type,omitempty"`
	GroupID string `json:"group_id,omitempty"`
}

type newAPIUploadAssetResult struct {
	AssetID       string `json:"asset_id"`
	GroupID       string `json:"group_id"`
	Status        string `json:"status"`
	FailureReason string `json:"failure_reason,omitempty"`
}

// ListAssets returns a page of assets in an asset group.
func (a *NewAPIAssetAdaptor) ListAssets(ctx context.Context, cfg *ProviderConfig, request *ListAssetsRequest) (*AssetList, error) {
	if request == nil {
		return nil, fmt.Errorf("list assets request is required")
	}
	return newAPIAssetCall[AssetList](ctx, a, cfg, http.MethodPost, "/v1/assets/list", request)
}

// GetAsset returns an asset by its local ID.
func (a *NewAPIAssetAdaptor) GetAsset(ctx context.Context, cfg *ProviderConfig, id int64) (*Asset, error) {
	return newAPIAssetCall[Asset](ctx, a, cfg, http.MethodGet, fmt.Sprintf("/v1/assets/%d", id), nil)
}

// UpdateAsset updates an asset by its local ID.
func (a *NewAPIAssetAdaptor) UpdateAsset(ctx context.Context, cfg *ProviderConfig, id int64, request *UpdateAssetRequest) (*Asset, error) {
	if request == nil {
		return nil, fmt.Errorf("update asset request is required")
	}
	return newAPIAssetCall[Asset](ctx, a, cfg, http.MethodPut, fmt.Sprintf("/v1/assets/%d", id), request)
}

// DeleteAsset deletes an asset by its local ID.
func (a *NewAPIAssetAdaptor) DeleteAsset(ctx context.Context, cfg *ProviderConfig, id int64) (*DeleteAssetResult, error) {
	return newAPIAssetCall[DeleteAssetResult](ctx, a, cfg, http.MethodDelete, fmt.Sprintf("/v1/assets/%d", id), nil)
}

type newAPIAssetResponse[T any] struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

func newAPIAssetCall[T any](ctx context.Context, adaptor *NewAPIAssetAdaptor, cfg *ProviderConfig, method, path string, payload interface{}) (*T, error) {
	endpoint, err := newAPIAssetEndpoint(cfg, path)
	if err != nil {
		return nil, err
	}

	var out newAPIAssetResponse[T]
	if err := adaptor.doJSON(ctx, cfg, method, endpoint, payload, &out); err != nil {
		return nil, err
	}
	if !out.Success {
		return nil, newAPIAssetBusinessError(out.Message)
	}
	return &out.Data, nil
}

func (a *NewAPIAssetAdaptor) doJSON(ctx context.Context, cfg *ProviderConfig, method, endpoint string, payload, out interface{}) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode asset API request: %w", err)
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return fmt.Errorf("create asset API request: %w", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if cfg != nil && cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
	if cfg != nil {
		for key, value := range cfg.Headers {
			req.Header.Set(key, value)
		}
	}

	resp, err := newAPIAssetHTTPClient(cfg).Do(req)
	if err != nil {
		return fmt.Errorf("call asset API: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read asset API response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return newAPIAssetResponseError(resp.StatusCode, raw)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode asset API response: %w", err)
	}
	return nil
}

func newAPIAssetEndpoint(cfg *ProviderConfig, path string) (string, error) {
	rawBaseURL := ""
	if cfg != nil {
		rawBaseURL = cfg.BaseURL
	}
	endpoint, err := url.Parse(strings.TrimSpace(rawBaseURL))
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return "", fmt.Errorf("asset API base URL must be an absolute http(s) URL: %q", rawBaseURL)
	}
	endpoint.Path = "/" + strings.TrimLeft(path, "/")
	endpoint.RawPath = ""
	endpoint.RawQuery = ""
	endpoint.Fragment = ""
	return endpoint.String(), nil
}

func newAPIAssetHTTPClient(cfg *ProviderConfig) *http.Client {
	if cfg != nil && cfg.HTTPClient != nil {
		return cfg.HTTPClient
	}
	client := &http.Client{}
	if cfg != nil && cfg.Timeout > 0 {
		client.Timeout = cfg.Timeout
	}
	return client
}

func newAPIAssetResponseError(statusCode int, raw []byte) error {
	var out struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &out) == nil && strings.TrimSpace(out.Message) != "" {
		return fmt.Errorf("asset API error: status=%d message=%s", statusCode, out.Message)
	}
	return fmt.Errorf("asset API error: status=%d body=%s", statusCode, strings.TrimSpace(string(raw)))
}

func newAPIAssetBusinessError(message string) error {
	if strings.TrimSpace(message) == "" {
		return fmt.Errorf("asset API request failed")
	}
	return fmt.Errorf("asset API error: message=%s", message)
}
