package celo

import (
	"encoding/json"
	"fmt"

	"github.com/defistate/defistate/differ"
	"github.com/defistate/defistate/engine"
	"github.com/defistate/defistate/patcher"
	token "github.com/defistate/defistate/protocols/erc20-token-system"
	poolregistry "github.com/defistate/defistate/protocols/pool-registry"
	uniswapv2 "github.com/defistate/defistate/protocols/uniswap-v2"
	uniswapv3 "github.com/defistate/defistate/protocols/uniswap-v3"
	"github.com/prometheus/client_golang/prometheus"
)

// StateOps encapsulates the core business logic for processing Celo DSE State.
//
// It acts as a unified facade for two critical operations:
// 1. Differ: Calculating the delta between two states (Used by the Server/Engine).
// 2. Patcher: Applying a delta to a previous state to reconstruct the present (Used by a Client).
type StateOps struct {
	*differ.StateDiffer
	*patcher.StatePatcher
}

// Logger defines a standard interface for structured, leveled logging.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

func NewStateOps(
	logger Logger,
	prometheusRegistry prometheus.Registerer,
) (*StateOps, error) {
	protocolDiffers := map[engine.ProtocolSchema]differ.ProtocolDiffer{
		token.TokenProtocolSchema: func(old, new any) (diff any, err error) {
			return token.Differ(old.([]token.TokenView), new.([]token.TokenView)), nil
		},
		poolregistry.PoolProtocolSchema: func(old, new any) (diff any, err error) {
			return poolregistry.Differ(old.(poolregistry.PoolRegistryView), new.(poolregistry.PoolRegistryView)), nil
		},
		poolregistry.TokenPoolProtocolSchema: func(old, new any) (diff any, err error) {
			return poolregistry.TokenPoolsRegistryDiffer(old.(*poolregistry.TokenPoolsRegistryView), new.(*poolregistry.TokenPoolsRegistryView)), nil
		},
		uniswapv2.UniswapV2ProtocolSchema: func(old, new any) (diff any, err error) {
			return uniswapv2.Differ(old.([]uniswapv2.PoolView), new.([]uniswapv2.PoolView)), nil
		},
		uniswapv3.UniswapV3ProtocolSchema: func(old, new any) (diff any, err error) {
			return uniswapv3.Differ(old.([]uniswapv3.PoolView), new.([]uniswapv3.PoolView)), nil
		},
	}

	protocolPatchers := map[engine.ProtocolSchema]patcher.PatcherFunc{
		token.TokenProtocolSchema: func(prevState, diff any) (newState any, err error) {
			return token.Patcher(prevState.([]token.TokenView), diff.(token.TokenSystemDiff))
		},
		poolregistry.PoolProtocolSchema: func(prevState, diff any) (newState any, err error) {
			return poolregistry.Patcher(prevState.(poolregistry.PoolRegistryView), diff.(poolregistry.PoolRegistryDiff))
		},
		poolregistry.TokenPoolProtocolSchema: func(prevState, diff any) (newState any, err error) {
			return poolregistry.TokenPoolRegistryPatcher(prevState.(*poolregistry.TokenPoolsRegistryView), diff.(poolregistry.TokenPoolRegistryDiff))
		},
		uniswapv2.UniswapV2ProtocolSchema: func(prevState, diff any) (newState any, err error) {
			return uniswapv2.Patcher(prevState.([]uniswapv2.PoolView), diff.(uniswapv2.UniswapV2SystemDiff))
		},
		uniswapv3.UniswapV3ProtocolSchema: func(prevState, diff any) (newState any, err error) {
			return uniswapv3.Patcher(prevState.([]uniswapv3.PoolView), diff.(uniswapv3.UniswapV3SystemDiff))
		},
	}

	stateDiffer, err := differ.NewStateDiffer(&differ.StateDifferConfig{
		ProtocolDiffers: protocolDiffers,
		Logger:          logger,
		Registry:        prometheusRegistry,
	})
	if err != nil {
		return nil, err
	}

	statePatcher, err := patcher.NewStatePatcher(&patcher.StatePatcherConfig{
		Patchers: protocolPatchers,
	})
	if err != nil {
		return nil, err
	}

	return &StateOps{
		StateDiffer:  stateDiffer,
		StatePatcher: statePatcher,
	}, nil

}

func (ops *StateOps) DecodeStateJSON(
	schema engine.ProtocolSchema,
	data json.RawMessage,
) (any, error) {
	switch schema {
	case token.TokenProtocolSchema:
		var typedData []token.TokenView
		err := json.Unmarshal(data, &typedData)
		if err != nil {
			return nil, err
		}
		return typedData, nil

	case poolregistry.PoolProtocolSchema:
		var typedData poolregistry.PoolRegistryView
		err := json.Unmarshal(data, &typedData)
		if err != nil {
			return nil, err
		}
		return typedData, nil
	case poolregistry.TokenPoolProtocolSchema:
		var typedData *poolregistry.TokenPoolsRegistryView
		err := json.Unmarshal(data, &typedData)
		if err != nil {
			return nil, err
		}
		return typedData, nil
	case uniswapv2.UniswapV2ProtocolSchema:
		var typedData []uniswapv2.PoolView
		err := json.Unmarshal(data, &typedData)
		if err != nil {
			return nil, err
		}
		return typedData, nil
	case uniswapv3.UniswapV3ProtocolSchema:
		var typedData []uniswapv3.PoolView
		err := json.Unmarshal(data, &typedData)
		if err != nil {
			return nil, err
		}
		return typedData, nil
	default:
		return nil, fmt.Errorf("unknown schema: %s", schema)

	}
}

func (ops *StateOps) DecodeStateDiffJSON(
	schema engine.ProtocolSchema,
	data json.RawMessage,
) (any, error) {
	switch schema {
	case token.TokenProtocolSchema:
		var typedData token.TokenSystemDiff
		err := json.Unmarshal(data, &typedData)
		if err != nil {
			return nil, err
		}
		return typedData, nil

	case poolregistry.PoolProtocolSchema:
		var typedData poolregistry.PoolRegistryDiff
		err := json.Unmarshal(data, &typedData)
		if err != nil {
			return nil, err
		}
		return typedData, nil
	case poolregistry.TokenPoolProtocolSchema:
		var typedData poolregistry.TokenPoolRegistryDiff
		err := json.Unmarshal(data, &typedData)
		if err != nil {
			return nil, err
		}
		return typedData, nil
	case uniswapv2.UniswapV2ProtocolSchema:
		var typedData uniswapv2.UniswapV2SystemDiff
		err := json.Unmarshal(data, &typedData)
		if err != nil {
			return nil, err
		}
		return typedData, nil
	case uniswapv3.UniswapV3ProtocolSchema:
		var typedData uniswapv3.UniswapV3SystemDiff
		err := json.Unmarshal(data, &typedData)
		if err != nil {
			return nil, err
		}
		return typedData, nil
	default:
		return nil, fmt.Errorf("unknown schema: %s", schema)

	}
}
