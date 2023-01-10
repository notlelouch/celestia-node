package state

import (
	"context"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	rpcclient "github.com/tendermint/tendermint/rpc/client"
	"google.golang.org/grpc"

	"github.com/celestiaorg/celestia-node/core"

	"github.com/celestiaorg/celestia-app/app"
	"github.com/celestiaorg/celestia-app/testutil/testnode"
	blobtypes "github.com/celestiaorg/celestia-app/x/payment/types"

	"github.com/celestiaorg/celestia-node/header"
)

func TestIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(IntegrationTestSuite))
}

type IntegrationTestSuite struct {
	suite.Suite

	cleanups []func() error
	accounts []string
	cctx     testnode.Context

	accessor *CoreAccessor
}

func (s *IntegrationTestSuite) SetupSuite() {
	if testing.Short() {
		s.T().Skip("skipping test in unit-tests")
	}
	s.T().Log("setting up integration test suite")

	cfg := core.DefaultTestConfig()
	s.cctx = core.StartTestNodeWithConfig(s.T(), cfg)
	s.accounts = cfg.Accounts

	signer := blobtypes.NewKeyringSigner(s.cctx.Keyring, s.accounts[0], s.cctx.ChainID)
	accessor := NewCoreAccessor(signer, localHeader{s.cctx.Client}, "", "", "")
	setClients(accessor, s.cctx.GRPCClient, s.cctx.Client)
	s.accessor = accessor

	// required to ensure the Head request is non-nil
	require.NoError(s.T(), s.cctx.WaitForNextBlock())
}

func setClients(ca *CoreAccessor, conn *grpc.ClientConn, abciCli rpcclient.ABCIClient) {
	ca.coreConn = conn
	// create the query client
	queryCli := banktypes.NewQueryClient(ca.coreConn)
	ca.queryCli = queryCli
	// create the staking query client
	stakingCli := stakingtypes.NewQueryClient(ca.coreConn)
	ca.stakingCli = stakingCli

	ca.rpcCli = abciCli
}

func (s *IntegrationTestSuite) TearDownSuite() {
	s.T().Log("tearing down integration test suite")
	require := s.Require()
	require.NoError(s.accessor.Stop(s.cctx.GoContext()))
	for _, c := range s.cleanups {
		err := c()
		require.NoError(err)
	}
}

func (s *IntegrationTestSuite) getAddress(acc string) sdk.Address {
	rec, err := s.cctx.Keyring.Key(acc)
	require.NoError(s.T(), err)

	addr, err := rec.GetAddress()
	require.NoError(s.T(), err)

	return addr
}

type localHeader struct {
	client rpcclient.Client
}

func (l localHeader) Head(ctx context.Context) (*header.ExtendedHeader, error) {
	latest, err := l.client.Block(ctx, nil)
	if err != nil {
		return nil, err
	}
	h := &header.ExtendedHeader{
		RawHeader: latest.Block.Header,
	}
	return h, nil
}

func (s *IntegrationTestSuite) TestGetBalance() {
	require := s.Require()
	expectedBal := sdk.NewCoin(app.BondDenom, sdk.NewInt(int64(99999999999999999)))
	for _, acc := range s.accounts {
		bal, err := s.accessor.BalanceForAddress(context.Background(), s.getAddress(acc))
		require.NoError(err)
		require.Equal(&expectedBal, bal)
	}
}