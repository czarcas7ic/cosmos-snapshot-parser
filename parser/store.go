package parser

import (
	db "github.com/cometbft/cometbft-db"
	tmstore "github.com/cometbft/cometbft/store"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/store/rootmulti"
	store "github.com/cosmos/cosmos-sdk/store/types"
	"github.com/cosmos/cosmos-sdk/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	capabilitytypes "github.com/cosmos/cosmos-sdk/x/capability/types"
	distrkeeper "github.com/cosmos/cosmos-sdk/x/distribution/keeper"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	evidencetypes "github.com/cosmos/cosmos-sdk/x/evidence/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	paramskeeper "github.com/cosmos/cosmos-sdk/x/params/keeper"
	paramstypes "github.com/cosmos/cosmos-sdk/x/params/types"
	slashingkeeper "github.com/cosmos/cosmos-sdk/x/slashing/keeper"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"
	icahosttypes "github.com/cosmos/ibc-go/v7/modules/apps/27-interchain-accounts/host/types"
	ibctransfertypes "github.com/cosmos/ibc-go/v7/modules/apps/transfer/types"
	"github.com/osmosis-labs/osmosis/v21/app"
	mintkeeper "github.com/osmosis-labs/osmosis/v21/x/mint/keeper"
	minttypes "github.com/osmosis-labs/osmosis/v21/x/mint/types"
	epochskeeper "github.com/osmosis-labs/osmosis/x/epochs/keeper"
	epochstypes "github.com/osmosis-labs/osmosis/x/epochs/types"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

func LoadDataStores(dbDir string, keys map[string]*store.KVStoreKey) (
	appStore *rootmulti.Store,
	blockStore *tmstore.BlockStore,
) {
	o := opt.Options{
		DisableSeeksCompaction: true,
	}
	// Get the application store from a directory
	appDB, err := db.NewGoLevelDBWithOpts("application", dbDir, &o)
	appStore = rootmulti.NewStore(appDB,
		nil,
	)
	if err != nil {
		panic(err)
	}

	// Get the block store from a directory
	blockStoreDB, err := db.NewGoLevelDBWithOpts("blockstore", dbDir, &o)
	if err != nil {
		panic(err)
	}
	blockStore = tmstore.NewBlockStore(blockStoreDB)

	for _, value := range keys {
		appStore.MountStoreWithDB(value, store.StoreTypeIAVL, nil)
	}

	// Load the latest version in the state
	err = appStore.LoadLatestVersion()
	if err != nil {
		panic(err)
	}

	return
}

func CreateKeepers(marshaler *codec.ProtoCodec) (
	pk *paramskeeper.Keeper,
	ak *authkeeper.AccountKeeper,
	bk *bankkeeper.BaseKeeper,
	sk *stakingkeeper.Keeper,
	mk *mintkeeper.Keeper,
	dk *distrkeeper.Keeper,
	slk *slashingkeeper.Keeper,
	keys map[string]*store.KVStoreKey,
) {

	// todo allow for other keys to be mounted
	keys = types.NewKVStoreKeys(
		authtypes.StoreKey,
		banktypes.StoreKey,
		stakingtypes.StoreKey,
		minttypes.StoreKey,
		distrtypes.StoreKey,
		slashingtypes.StoreKey,
		govtypes.StoreKey,
		paramstypes.StoreKey,
		icahosttypes.StoreKey,
		upgradetypes.StoreKey,
		evidencetypes.StoreKey,
		ibctransfertypes.StoreKey,
		capabilitytypes.StoreKey,
	)

	tkeys := sdk.NewTransientStoreKeys(paramstypes.TStoreKey)
	// module account permissions
	maccPerms := map[string][]string{
		authtypes.FeeCollectorName:     nil,
		distrtypes.ModuleName:          nil,
		minttypes.ModuleName:           {authtypes.Minter},
		stakingtypes.BondedPoolName:    {authtypes.Burner, authtypes.Staking},
		stakingtypes.NotBondedPoolName: {authtypes.Burner, authtypes.Staking},
		govtypes.ModuleName:            {authtypes.Burner},
	}

	allowedReceivingModAcc := map[string]bool{}

	blockedAddrs := make(map[string]bool)
	for acc := range maccPerms {
		blockedAddrs[authtypes.NewModuleAddress(acc).String()] = !allowedReceivingModAcc[acc]
	}

	EpochsKeeper := epochskeeper.NewKeeper(keys[epochstypes.StoreKey])

	cdc := app.MakeEncodingConfig().Amino

	paramsKeeper := paramskeeper.NewKeeper(
		marshaler,
		nil,
		keys[paramstypes.StoreKey],
		tkeys[paramstypes.StoreKey],
	)

	AccountKeeper := authkeeper.NewAccountKeeper(
		marshaler,
		keys[authtypes.StoreKey],
		authtypes.ProtoBaseAccount,
		maccPerms,
		"osmo",
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)

	BankKeeper := bankkeeper.NewBaseKeeper(
		marshaler,
		keys[banktypes.StoreKey],
		AccountKeeper,
		blockedAddrs,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)

	StakingKeeper := stakingkeeper.NewKeeper(
		marshaler,
		keys[stakingtypes.StoreKey],
		AccountKeeper,
		BankKeeper,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)

	DistrKeeper := distrkeeper.NewKeeper(
		marshaler,
		keys[distrtypes.StoreKey],
		AccountKeeper,
		BankKeeper,
		StakingKeeper,
		authtypes.FeeCollectorName,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)

	MintKeeper := mintkeeper.NewKeeper(
		keys[minttypes.StoreKey],
		paramsKeeper.Subspace(minttypes.ModuleName),
		AccountKeeper,
		BankKeeper,
		DistrKeeper,
		EpochsKeeper,
		authtypes.FeeCollectorName,
	)

	SlashingKeeper := slashingkeeper.NewKeeper(
		marshaler,
		cdc,
		keys[slashingtypes.StoreKey],
		StakingKeeper,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)

	return &paramsKeeper,
		&AccountKeeper,
		&BankKeeper,
		StakingKeeper,
		&MintKeeper,
		&DistrKeeper,
		&SlashingKeeper,
		keys
}
