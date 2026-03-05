package config

import (
	"math/big"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the application's configuration settings.
type Config struct {
	ChainID       *big.Int            `yaml:"chain_id" json:"chain_id"`
	Chain         string              `yaml:"chain" json:"chain"`
	ClientManager ClientManagerConfig `yaml:"client_manager" json:"client_manager"`
	Engine        EngineConfig        `yaml:"engine" json:"engine"`
	Fork          ForkConfig          `yaml:"fork" json:"fork"`
}

// ClientManagerConfig holds the settings for the main RPC client pool.
type ClientManagerConfig struct {
	RPCURLs               []string `yaml:"rpc_urls" json:"rpc_urls"`
	MaxConcurrentRequests int      `yaml:"max_concurrent_requests" json:"max_concurrent_requests"`
}

// EngineConfig holds the settings for the core DeFi state engine.
type EngineConfig struct {
	MaxWaitUntilSync time.Duration `yaml:"max_wait_until_sync" json:"max_wait_until_sync"`
}

type ForkConfig struct {
	RPCURL         string                    `yaml:"rpc_url" json:"rpc_url"`
	TokenOverrides map[string]FeeGasOverride `yaml:"token_overrides"` // tokenOverrides allows users to specify custom token fee and gas for specific addresses
}

type FeeGasOverride struct {
	Gas                     uint `yaml:"gas"`
	FeeOnTransferPercentage uint `yaml:"fee_on_transfer_percentage"`
}

// LoadConfig reads a configuration file from the given path and unmarshals it
// into a Config struct.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
