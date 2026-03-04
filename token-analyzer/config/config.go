// Package config provides structures and functions for loading and managing
// application configuration from a file.
package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the top-level structure for the application's configuration.
type Config struct {
	APIServer APIServerConfig `yaml:"api_server"`
	Chains    []ChainConfig   `yaml:"chains"`
}

// APIServerConfig holds the settings for the main HTTP service.
type APIServerConfig struct {
	Addr string `yaml:"addr"`
}

// ChainConfig holds all the configuration specific to a single blockchain.
type ChainConfig struct {
	Name               string                   `yaml:"name"`
	ChainID            uint64                   `yaml:"chain_id"`
	IsEnabled          bool                     `yaml:"is_enabled"`
	ClientManager      ClientManagerConfig      `yaml:"client_manager"`
	FeeAndGasRequester FeeAndGasRequesterConfig `yaml:"fee_and_gas_requester"`
	ERC20Analyzer      ERC20AnalyzerConfig      `yaml:"erc20_analyzer"`
}

// ClientManagerConfig holds the settings for the main RPC client pool.
type ClientManagerConfig struct {
	RPCURLs               []string `yaml:"rpc_urls"`
	MaxConcurrentRequests int      `yaml:"max_concurrent_requests"`
}

// FeeAndGasRequesterConfig holds the settings for the fork RPC client.
type FeeAndGasRequesterConfig struct {
	ForkRPCURL            string `yaml:"fork_rpc_url"`
	MaxConcurrentRequests int    `yaml:"max_concurrent_requests"`
}

// ERC20AnalyzerConfig holds the settings for the erc20analyzer component.
type ERC20AnalyzerConfig struct {
	MinTokenUpdateInterval   time.Duration        `yaml:"min_token_update_interval"`
	FeeAndGasUpdateFrequency time.Duration        `yaml:"fee_and_gas_update_frequency"`
	VolumeAnalyzer           VolumeAnalyzerConfig `yaml:"volume_analyzer"`
}

// VolumeAnalyzerConfig holds the specific settings for the volume-based holder analyzer.
type VolumeAnalyzerConfig struct {
	ExpiryCheckFrequency time.Duration `yaml:"expiry_check_frequency"`
	RecordStaleDuration  time.Duration `yaml:"record_stale_duration"`
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
