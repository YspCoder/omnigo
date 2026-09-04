package omnigo

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/YspCoder/omnigo/adapter"
)

// AssetProvider identifies an asset management service provider.
type AssetProvider string

const (
	// AssetProviderNewAPI uses newapi asset management endpoints.
	AssetProviderNewAPI AssetProvider = "newapi"
	// AssetProviderNewAPIArk uses the NewAPI Ark Action asset endpoints.
	AssetProviderNewAPIArk AssetProvider = "newapi_ark"

	NewAPIArkAssetTypeImage = adapter.NewAPIArkAssetTypeImage

	NewAPIArkAssetStatusProcessing = adapter.NewAPIArkAssetStatusProcessing
	NewAPIArkAssetStatusActive     = adapter.NewAPIArkAssetStatusActive
	NewAPIArkAssetStatusFailed     = adapter.NewAPIArkAssetStatusFailed
)

// Asset management types are re-exported so callers only need the omnigo package.
type (
	AssetConfig             = adapter.ProviderConfig
	AssetGroup              = adapter.AssetGroup
	CreateAssetGroupRequest = adapter.CreateAssetGroupRequest
	ListAssetGroupsRequest  = adapter.ListAssetGroupsRequest
	AssetGroupList          = adapter.AssetGroupList
	UpdateAssetGroupRequest = adapter.UpdateAssetGroupRequest
	Asset                   = adapter.Asset
	CreateAssetRequest      = adapter.CreateAssetRequest
	ListAssetsRequest       = adapter.ListAssetsRequest
	AssetList               = adapter.AssetList
	UpdateAssetRequest      = adapter.UpdateAssetRequest
	DeleteAssetResult       = adapter.DeleteAssetResult

	NewAPIArkResponseMetadata = adapter.NewAPIArkResponseMetadata
	NewAPIArkErrorDetail      = adapter.NewAPIArkErrorDetail
	NewAPIArkAssetAPIError    = adapter.NewAPIArkAssetAPIError
)

// AssetClient manages asset groups and assets for one configured provider.
type AssetClient interface {
	CreateAssetGroup(context.Context, *CreateAssetGroupRequest) (*AssetGroup, error)
	ListAssetGroups(context.Context, *ListAssetGroupsRequest) (*AssetGroupList, error)
	GetAssetGroup(context.Context, any) (*AssetGroup, error)
	UpdateAssetGroup(context.Context, any, *UpdateAssetGroupRequest) (*AssetGroup, error)
	DeleteAssetGroup(context.Context, any) (*DeleteAssetResult, error)

	CreateAsset(context.Context, *CreateAssetRequest) (*Asset, error)
	ListAssets(context.Context, *ListAssetsRequest) (*AssetList, error)
	GetAsset(context.Context, any) (*Asset, error)
	UpdateAsset(context.Context, any, *UpdateAssetRequest) (*Asset, error)
	DeleteAsset(context.Context, any) (*DeleteAssetResult, error)
}

type assetClient struct {
	adaptor    adapter.AssetAdaptor
	arkAdaptor *adapter.NewAPIArkAssetAdaptor
	config     *adapter.ProviderConfig
}

// NewAssetClient creates an asset management client for a provider.
func NewAssetClient(provider AssetProvider, config *AssetConfig) (AssetClient, error) {
	if config == nil {
		return nil, fmt.Errorf("asset config is required")
	}

	var adaptor adapter.AssetAdaptor
	var arkAdaptor *adapter.NewAPIArkAssetAdaptor
	switch provider {
	case AssetProviderNewAPI:
		adaptor = &adapter.NewAPIAssetAdaptor{}
	case AssetProviderNewAPIArk:
		if strings.TrimSpace(config.APIKey) == "" {
			return nil, fmt.Errorf("NewAPI Ark asset API key is required")
		}
		if strings.TrimSpace(config.BaseURL) == "" {
			return nil, fmt.Errorf("NewAPI Ark asset API base URL is required")
		}
		arkAdaptor = &adapter.NewAPIArkAssetAdaptor{}
	default:
		return nil, fmt.Errorf("unknown asset provider: %s", provider)
	}

	return &assetClient{adaptor: adaptor, arkAdaptor: arkAdaptor, config: config}, nil
}

func (c *assetClient) CreateAssetGroup(ctx context.Context, request *CreateAssetGroupRequest) (*AssetGroup, error) {
	if c.arkAdaptor == nil {
		return c.adaptor.CreateAssetGroup(ctx, c.config, request)
	}
	if request == nil {
		return nil, fmt.Errorf("create asset group request is required")
	}
	result, err := c.arkAdaptor.CreateAssetGroup(ctx, c.config, &adapter.NewAPIArkCreateAssetGroupRequest{
		Name:        request.GroupName,
		Description: request.Description,
	})
	if err != nil {
		return nil, err
	}
	return &AssetGroup{
		CloudGroupID:     result.ID,
		GroupType:        "AIGC",
		GroupName:        request.GroupName,
		Description:      request.Description,
		ResponseMetadata: newAPIArkMetadata(result.ResponseMetadata),
	}, nil
}

func (c *assetClient) ListAssetGroups(ctx context.Context, request *ListAssetGroupsRequest) (*AssetGroupList, error) {
	if c.arkAdaptor == nil {
		return c.adaptor.ListAssetGroups(ctx, c.config, request)
	}
	if request == nil {
		return nil, fmt.Errorf("list asset groups request is required")
	}
	filter := &adapter.NewAPIArkAssetGroupFilter{
		GroupIDs:  request.GroupIDs,
		Name:      request.GroupName,
		GroupType: request.GroupType,
	}
	result, err := c.arkAdaptor.ListAssetGroups(ctx, c.config, &adapter.NewAPIArkListAssetGroupsRequest{
		Filter:     filter,
		PageNumber: request.PageNo,
		PageSize:   request.PageSize,
		SortBy:     request.SortBy,
		SortOrder:  request.SortOrder,
	})
	if err != nil {
		return nil, err
	}
	groups := make([]AssetGroup, 0, len(result.Items))
	for _, group := range result.Items {
		groups = append(groups, assetGroupFromNewAPIArk(group))
	}
	return &AssetGroupList{
		Items:            groups,
		Total:            result.TotalCount,
		PageNo:           result.PageNumber,
		PageSize:         result.PageSize,
		ResponseMetadata: newAPIArkMetadata(result.ResponseMetadata),
	}, nil
}

func (c *assetClient) GetAssetGroup(ctx context.Context, id any) (*AssetGroup, error) {
	if c.arkAdaptor == nil {
		localID, err := newAPILocalID(id)
		if err != nil {
			return nil, err
		}
		return c.adaptor.GetAssetGroup(ctx, c.config, localID)
	}
	resourceID, err := newAPIArkResourceID(id)
	if err != nil {
		return nil, err
	}
	result, err := c.arkAdaptor.GetAssetGroup(ctx, c.config, &adapter.NewAPIArkGetAssetGroupRequest{ID: resourceID})
	if err != nil {
		return nil, err
	}
	group := assetGroupFromNewAPIArk(*result)
	group.ResponseMetadata = newAPIArkMetadata(result.ResponseMetadata)
	return &group, nil
}

func (c *assetClient) UpdateAssetGroup(ctx context.Context, id any, request *UpdateAssetGroupRequest) (*AssetGroup, error) {
	if c.arkAdaptor == nil {
		localID, err := newAPILocalID(id)
		if err != nil {
			return nil, err
		}
		return c.adaptor.UpdateAssetGroup(ctx, c.config, localID, request)
	}
	if request == nil {
		return nil, fmt.Errorf("update asset group request is required")
	}
	resourceID, err := newAPIArkResourceID(id)
	if err != nil {
		return nil, err
	}
	result, err := c.arkAdaptor.UpdateAssetGroup(ctx, c.config, &adapter.NewAPIArkUpdateAssetGroupRequest{
		ID:          resourceID,
		Name:        request.GroupName,
		Description: request.Description,
	})
	if err != nil {
		return nil, err
	}
	group := &AssetGroup{CloudGroupID: result.ID, ResponseMetadata: newAPIArkMetadata(result.ResponseMetadata)}
	if request.GroupName != nil {
		group.GroupName = *request.GroupName
	}
	if request.Description != nil {
		group.Description = *request.Description
	}
	return group, nil
}

func (c *assetClient) DeleteAssetGroup(ctx context.Context, id any) (*DeleteAssetResult, error) {
	if c.arkAdaptor == nil {
		localID, err := newAPILocalID(id)
		if err != nil {
			return nil, err
		}
		return c.adaptor.DeleteAssetGroup(ctx, c.config, localID)
	}
	resourceID, err := newAPIArkResourceID(id)
	if err != nil {
		return nil, err
	}
	result, err := c.arkAdaptor.DeleteAssetGroup(ctx, c.config, &adapter.NewAPIArkDeleteAssetGroupRequest{ID: resourceID})
	if err != nil {
		return nil, err
	}
	return &DeleteAssetResult{Deleted: true, ResponseMetadata: newAPIArkMetadata(result.ResponseMetadata)}, nil
}

func (c *assetClient) CreateAsset(ctx context.Context, request *CreateAssetRequest) (*Asset, error) {
	if c.arkAdaptor == nil {
		return c.adaptor.CreateAsset(ctx, c.config, request)
	}
	if request == nil {
		return nil, fmt.Errorf("create asset request is required")
	}
	purpose := strings.TrimSpace(request.Purpose)
	if purpose == "" {
		purpose = request.AssetName
	}
	result, err := c.arkAdaptor.CreateAsset(ctx, c.config, &adapter.NewAPIArkCreateAssetRequest{
		Purpose:   purpose,
		URL:       request.AssetURL,
		AssetType: request.AssetType,
		GroupID:   request.CloudGroupID,
	})
	if err != nil {
		return nil, err
	}
	return &Asset{
		CloudAssetID:     result.ID,
		CloudGroupID:     request.CloudGroupID,
		AssetType:        request.AssetType,
		AssetName:        request.AssetName,
		AssetURL:         request.AssetURL,
		ResponseMetadata: newAPIArkMetadata(result.ResponseMetadata),
	}, nil
}

func (c *assetClient) ListAssets(ctx context.Context, request *ListAssetsRequest) (*AssetList, error) {
	if c.arkAdaptor == nil {
		return c.adaptor.ListAssets(ctx, c.config, request)
	}
	if request == nil {
		return nil, fmt.Errorf("list assets request is required")
	}
	groupIDs := request.GroupIDs
	if len(groupIDs) == 0 && strings.TrimSpace(request.CloudGroupID) != "" {
		groupIDs = []string{request.CloudGroupID}
	}
	result, err := c.arkAdaptor.ListAssets(ctx, c.config, &adapter.NewAPIArkListAssetsRequest{
		Filter: &adapter.NewAPIArkAssetFilter{
			GroupIDs:  groupIDs,
			GroupType: request.GroupType,
			Name:      request.AssetName,
		},
		PageNumber: request.PageNo,
		PageSize:   request.PageSize,
		SortBy:     request.SortBy,
		SortOrder:  request.SortOrder,
	})
	if err != nil {
		return nil, err
	}
	assets := make([]Asset, 0, len(result.Items))
	for _, asset := range result.Items {
		assets = append(assets, assetFromNewAPIArk(asset))
	}
	return &AssetList{
		Items:            assets,
		Total:            result.TotalCount,
		PageNo:           result.PageNumber,
		PageSize:         result.PageSize,
		ResponseMetadata: newAPIArkMetadata(result.ResponseMetadata),
	}, nil
}

func (c *assetClient) GetAsset(ctx context.Context, id any) (*Asset, error) {
	if c.arkAdaptor == nil {
		localID, err := newAPILocalID(id)
		if err != nil {
			return nil, err
		}
		return c.adaptor.GetAsset(ctx, c.config, localID)
	}
	resourceID, err := newAPIArkResourceID(id)
	if err != nil {
		return nil, err
	}
	result, err := c.arkAdaptor.GetAsset(ctx, c.config, &adapter.NewAPIArkGetAssetRequest{ID: resourceID})
	if err != nil {
		return nil, err
	}
	asset := assetFromNewAPIArk(*result)
	asset.ResponseMetadata = newAPIArkMetadata(result.ResponseMetadata)
	return &asset, nil
}

func (c *assetClient) UpdateAsset(ctx context.Context, id any, request *UpdateAssetRequest) (*Asset, error) {
	if c.arkAdaptor == nil {
		localID, err := newAPILocalID(id)
		if err != nil {
			return nil, err
		}
		return c.adaptor.UpdateAsset(ctx, c.config, localID, request)
	}
	if request == nil {
		return nil, fmt.Errorf("update asset request is required")
	}
	resourceID, err := newAPIArkResourceID(id)
	if err != nil {
		return nil, err
	}
	result, err := c.arkAdaptor.UpdateAsset(ctx, c.config, &adapter.NewAPIArkUpdateAssetRequest{ID: resourceID, Name: request.AssetName})
	if err != nil {
		return nil, err
	}
	return &Asset{
		CloudAssetID:     result.ID,
		AssetName:        request.AssetName,
		ResponseMetadata: newAPIArkMetadata(result.ResponseMetadata),
	}, nil
}

func (c *assetClient) DeleteAsset(ctx context.Context, id any) (*DeleteAssetResult, error) {
	if c.arkAdaptor == nil {
		localID, err := newAPILocalID(id)
		if err != nil {
			return nil, err
		}
		return c.adaptor.DeleteAsset(ctx, c.config, localID)
	}
	resourceID, err := newAPIArkResourceID(id)
	if err != nil {
		return nil, err
	}
	result, err := c.arkAdaptor.DeleteAsset(ctx, c.config, &adapter.NewAPIArkDeleteAssetRequest{ID: resourceID})
	if err != nil {
		return nil, err
	}
	return &DeleteAssetResult{Deleted: true, ResponseMetadata: newAPIArkMetadata(result.ResponseMetadata)}, nil
}

func assetGroupFromNewAPIArk(group adapter.NewAPIArkAssetGroup) AssetGroup {
	return AssetGroup{
		CloudGroupID: group.ID,
		GroupType:    group.GroupType,
		GroupName:    group.Name,
		Description:  group.Description,
		CreateTime:   group.CreateTime,
		UpdateTime:   group.UpdateTime,
		ProjectName:  group.ProjectName,
	}
}

func assetFromNewAPIArk(asset adapter.NewAPIArkAsset) Asset {
	return Asset{
		CloudAssetID: asset.ID,
		CloudGroupID: asset.GroupID,
		AssetType:    asset.AssetType,
		AssetStatus:  asset.Status,
		AssetName:    asset.Name,
		AssetURL:     asset.URL,
		CreateTime:   asset.CreateTime,
		UpdateTime:   asset.UpdateTime,
		ProjectName:  asset.ProjectName,
		Moderation:   asset.Moderation,
		Error:        asset.Error,
	}
}

func newAPIArkMetadata(metadata adapter.NewAPIArkResponseMetadata) *adapter.NewAPIArkResponseMetadata {
	return &metadata
}

func newAPIArkResourceID(id any) (string, error) {
	value, ok := id.(string)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("NewAPI Ark asset resource ID must be a non-empty string")
	}
	return strings.TrimSpace(value), nil
}

func newAPILocalID(id any) (int64, error) {
	switch value := id.(type) {
	case int:
		return int64(value), nil
	case int8:
		return int64(value), nil
	case int16:
		return int64(value), nil
	case int32:
		return int64(value), nil
	case int64:
		return value, nil
	case uint:
		if uint64(value) <= math.MaxInt64 {
			return int64(value), nil
		}
	case uint8:
		return int64(value), nil
	case uint16:
		return int64(value), nil
	case uint32:
		return int64(value), nil
	case uint64:
		if value <= math.MaxInt64 {
			return int64(value), nil
		}
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err == nil {
			return parsed, nil
		}
	}
	return 0, fmt.Errorf("newapi asset local ID must be an integer")
}
