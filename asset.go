package omnigo

import (
	"context"
	"fmt"

	"github.com/YspCoder/omnigo/adapter"
)

// AssetProvider identifies an asset management service provider.
type AssetProvider string

const (
	// AssetProviderNewAPI uses newapi asset management endpoints.
	AssetProviderNewAPI AssetProvider = "newapi"
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
)

// AssetClient manages asset groups and assets for one configured provider.
type AssetClient interface {
	CreateAssetGroup(context.Context, *CreateAssetGroupRequest) (*AssetGroup, error)
	ListAssetGroups(context.Context, *ListAssetGroupsRequest) (*AssetGroupList, error)
	GetAssetGroup(context.Context, int64) (*AssetGroup, error)
	UpdateAssetGroup(context.Context, int64, *UpdateAssetGroupRequest) (*AssetGroup, error)
	DeleteAssetGroup(context.Context, int64) (*DeleteAssetResult, error)

	CreateAsset(context.Context, *CreateAssetRequest) (*Asset, error)
	ListAssets(context.Context, *ListAssetsRequest) (*AssetList, error)
	GetAsset(context.Context, int64) (*Asset, error)
	UpdateAsset(context.Context, int64, *UpdateAssetRequest) (*Asset, error)
	DeleteAsset(context.Context, int64) (*DeleteAssetResult, error)
}

type assetClient struct {
	adaptor adapter.AssetAdaptor
	config  *adapter.ProviderConfig
}

// NewAssetClient creates an asset management client for a provider.
func NewAssetClient(provider AssetProvider, config *AssetConfig) (AssetClient, error) {
	if config == nil {
		return nil, fmt.Errorf("asset config is required")
	}

	var adaptor adapter.AssetAdaptor
	switch provider {
	case AssetProviderNewAPI:
		adaptor = &adapter.NewAPIAssetAdaptor{}
	default:
		return nil, fmt.Errorf("unknown asset provider: %s", provider)
	}

	return &assetClient{adaptor: adaptor, config: config}, nil
}

func (c *assetClient) CreateAssetGroup(ctx context.Context, request *CreateAssetGroupRequest) (*AssetGroup, error) {
	return c.adaptor.CreateAssetGroup(ctx, c.config, request)
}

func (c *assetClient) ListAssetGroups(ctx context.Context, request *ListAssetGroupsRequest) (*AssetGroupList, error) {
	return c.adaptor.ListAssetGroups(ctx, c.config, request)
}

func (c *assetClient) GetAssetGroup(ctx context.Context, id int64) (*AssetGroup, error) {
	return c.adaptor.GetAssetGroup(ctx, c.config, id)
}

func (c *assetClient) UpdateAssetGroup(ctx context.Context, id int64, request *UpdateAssetGroupRequest) (*AssetGroup, error) {
	return c.adaptor.UpdateAssetGroup(ctx, c.config, id, request)
}

func (c *assetClient) DeleteAssetGroup(ctx context.Context, id int64) (*DeleteAssetResult, error) {
	return c.adaptor.DeleteAssetGroup(ctx, c.config, id)
}

func (c *assetClient) CreateAsset(ctx context.Context, request *CreateAssetRequest) (*Asset, error) {
	return c.adaptor.CreateAsset(ctx, c.config, request)
}

func (c *assetClient) ListAssets(ctx context.Context, request *ListAssetsRequest) (*AssetList, error) {
	return c.adaptor.ListAssets(ctx, c.config, request)
}

func (c *assetClient) GetAsset(ctx context.Context, id int64) (*Asset, error) {
	return c.adaptor.GetAsset(ctx, c.config, id)
}

func (c *assetClient) UpdateAsset(ctx context.Context, id int64, request *UpdateAssetRequest) (*Asset, error) {
	return c.adaptor.UpdateAsset(ctx, c.config, id, request)
}

func (c *assetClient) DeleteAsset(ctx context.Context, id int64) (*DeleteAssetResult, error) {
	return c.adaptor.DeleteAsset(ctx, c.config, id)
}
