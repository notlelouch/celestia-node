package share

import (
	"context"

	"github.com/tendermint/tendermint/types"

	"github.com/celestiaorg/nmt"
	"github.com/celestiaorg/rsmt2d"

	"github.com/celestiaorg/celestia-node/header"
	headerServ "github.com/celestiaorg/celestia-node/nodebuilder/header"
	"github.com/celestiaorg/celestia-node/share"
	"github.com/celestiaorg/celestia-node/share/eds"
	"github.com/celestiaorg/celestia-node/share/shwap"
)

var _ Module = (*API)(nil)

// GetRangeResult wraps the return value of the GetRange endpoint
// because Json-RPC doesn't support more than two return values.
type GetRangeResult struct {
	Shares []share.Share
	Proof  *types.ShareProof
}

// Module provides access to any data square or block share on the network.
//
// All Get methods provided on Module follow the following flow:
//  1. Check local storage for the requested Share.
//  2. If exists
//     * Load from disk
//     * Return
//  3. If not
//     * Find provider on the network
//     * Fetch the Share from the provider
//     * Store the Share
//     * Return
//
// Any method signature changed here needs to also be changed in the API struct.
//
//go:generate mockgen -destination=mocks/api.go -package=mocks . Module
type Module interface {
	// SharesAvailable subjectively validates if Shares committed to the given
	// ExtendedHeader are available on the Network.
	SharesAvailable(context.Context, *header.ExtendedHeader) error
	// GetShare gets a Share by coordinates in EDS.
	GetShare(ctx context.Context, header *header.ExtendedHeader, row, col int) (share.Share, error)
	// GetEDS gets the full EDS identified by the given extended header.
	GetEDS(ctx context.Context, header *header.ExtendedHeader) (*rsmt2d.ExtendedDataSquare, error)
	// GetSharesByNamespace gets all shares from an EDS within the given namespace.
	// Shares are returned in a row-by-row order if the namespace spans multiple rows.
	GetSharesByNamespace(
		ctx context.Context, header *header.ExtendedHeader, namespace share.Namespace,
	) (NamespacedShares, error)
	// GetRange gets a list of shares and their corresponding proof.
	GetRange(ctx context.Context, height uint64, start, end int) (*GetRangeResult, error)
}

// API is a wrapper around Module for the RPC.
type API struct {
	Internal struct {
		SharesAvailable func(context.Context, *header.ExtendedHeader) error `perm:"read"`
		GetShare        func(
			ctx context.Context,
			header *header.ExtendedHeader,
			row, col int,
		) (share.Share, error) `perm:"read"`
		GetEDS func(
			ctx context.Context,
			header *header.ExtendedHeader,
		) (*rsmt2d.ExtendedDataSquare, error) `perm:"read"`
		GetSharesByNamespace func(
			ctx context.Context,
			header *header.ExtendedHeader,
			namespace share.Namespace,
		) (NamespacedShares, error) `perm:"read"`
		GetRange func(
			ctx context.Context,
			height uint64,
			start, end int,
		) (*GetRangeResult, error) `perm:"read"`
	}
}

func (api *API) SharesAvailable(ctx context.Context, header *header.ExtendedHeader) error {
	return api.Internal.SharesAvailable(ctx, header)
}

func (api *API) GetShare(ctx context.Context, header *header.ExtendedHeader, row, col int) (share.Share, error) {
	return api.Internal.GetShare(ctx, header, row, col)
}

func (api *API) GetEDS(ctx context.Context, header *header.ExtendedHeader) (*rsmt2d.ExtendedDataSquare, error) {
	return api.Internal.GetEDS(ctx, header)
}

func (api *API) GetRange(ctx context.Context, height uint64, start, end int) (*GetRangeResult, error) {
	return api.Internal.GetRange(ctx, height, start, end)
}

func (api *API) GetSharesByNamespace(
	ctx context.Context,
	header *header.ExtendedHeader,
	namespace share.Namespace,
) (NamespacedShares, error) {
	return api.Internal.GetSharesByNamespace(ctx, header, namespace)
}

type module struct {
	shwap.Getter
	share.Availability
	hs headerServ.Module
}

func (m module) SharesAvailable(ctx context.Context, header *header.ExtendedHeader) error {
	return m.Availability.SharesAvailable(ctx, header)
}

func (m module) GetRange(ctx context.Context, height uint64, start, end int) (*GetRangeResult, error) {
	extendedHeader, err := m.hs.GetByHeight(ctx, height)
	if err != nil {
		return nil, err
	}
	extendedDataSquare, err := m.GetEDS(ctx, extendedHeader)
	if err != nil {
		return nil, err
	}

	proof, err := eds.ProveShares(extendedDataSquare, start, end)
	if err != nil {
		return nil, err
	}
	return &GetRangeResult{
		extendedDataSquare.FlattenedODS()[start:end],
		proof,
	}, nil
}

func (m module) GetSharesByNamespace(
	ctx context.Context,
	header *header.ExtendedHeader,
	namespace share.Namespace,
) (NamespacedShares, error) {
	nd, err := m.Getter.GetSharesByNamespace(ctx, header, namespace)
	if err != nil {
		return nil, err
	}
	return convertToNamespacedShares(nd), nil
}

// NamespacedShares represents all shares with proofs within a specific namespace of an EDS.
// This is a copy of the share.NamespacedShares type, that is used to avoid breaking changes
// in the API.
type NamespacedShares []NamespacedRow

// NamespacedRow represents all shares with proofs within a specific namespace of a single EDS row.
type NamespacedRow struct {
	Shares []share.Share `json:"shares"`
	Proof  *nmt.Proof    `json:"proof"`
}

// Flatten returns the concatenated slice of all NamespacedRow shares.
func (ns NamespacedShares) Flatten() []share.Share {
	var shares []share.Share
	for _, row := range ns {
		shares = append(shares, row.Shares...)
	}
	return shares
}

func convertToNamespacedShares(nd shwap.NamespaceData) NamespacedShares {
	ns := make(NamespacedShares, 0, len(nd))
	for _, row := range nd {
		ns = append(ns, NamespacedRow{
			Shares: row.Shares,
			Proof:  row.Proof,
		})
	}
	return ns
}
