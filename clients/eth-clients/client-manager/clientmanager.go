package clientmanager

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"sync/atomic"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	"github.com/ethereum/go-ethereum/core/types"
)

var defaultPreferredClientMaxLatencyMultiplier = 3

type ClientManagerConfig struct {
	ClientConfig                        *ClientConfig
	PreferredClientMaxLatencyMultiplier int
	Logger                              Logger
}

type NewBlockNotification struct {
	Block *types.Block
}

func (config *ClientManagerConfig) applyDefaults() {
	if config.PreferredClientMaxLatencyMultiplier == 0 {
		config.PreferredClientMaxLatencyMultiplier = defaultPreferredClientMaxLatencyMultiplier
	}

	if config.Logger == nil {
		config.Logger = defaultLogger
	}

	if config.ClientConfig != nil && config.ClientConfig.Logger == nil {
		config.ClientConfig.Logger = config.Logger
	}
}

type ClientManager struct {
	clients                             []*Client
	counter                             int
	preferredClientMaxLatencyMultiplier int
	mu                                  sync.RWMutex
	logger                              Logger
	closed                              atomic.Bool
}

// NewClientManager creates a new ClientManager, dialing each provided endpoint.
func NewClientManager(
	ctx context.Context, // caller controls lifetime
	endpoints []string,
	config *ClientManagerConfig,
) (*ClientManager, error) {
	if len(endpoints) == 0 {
		return nil, errors.New("no endpoints provided")
	}

	// Dial clients with the same parent ctx
	clients := make([]*Client, 0, len(endpoints))
	for _, ep := range endpoints {
		c, err := NewClientFromDial(
			ctx,
			ep,
			config.ClientConfig,
		)
		if err != nil {
			return nil, fmt.Errorf("dial %s failed with error: %v", ep, err)
		}
		clients = append(clients, c)
	}

	config.applyDefaults()
	cm := &ClientManager{
		clients:                             clients,
		preferredClientMaxLatencyMultiplier: config.PreferredClientMaxLatencyMultiplier,
		logger:                              config.Logger,
	}
	return cm, nil
}

func NewClientManagerFromClients(
	clients []*Client,
	config *ClientManagerConfig,
) (*ClientManager, error) {
	if len(clients) == 0 {
		return nil, errors.New("no clients provided")
	}
	config.applyDefaults()
	cm := &ClientManager{
		clients:                             clients,
		preferredClientMaxLatencyMultiplier: config.PreferredClientMaxLatencyMultiplier,
		logger:                              config.Logger,
	}
	return cm, nil
}

// getClient returns a Client using a round-robin strategy.
func (cm *ClientManager) getClient() *Client {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	client := cm.clients[cm.counter]
	cm.counter = (cm.counter + 1) % len(cm.clients)
	return client
}

// GetClient returns an Ethereum client, preferring one that is healthy and up-to-date.
func (cm *ClientManager) GetClient() (ethclients.ETHClient, error) {
	// get latest known block
	latestBlockNumber := uint64(0)
	for _, c := range cm.clients {
		if c.LatestBlockNumber() > latestBlockNumber {
			latestBlockNumber = c.LatestBlockNumber()
		}
	}

	for i := 0; i < len(cm.clients); i++ {
		client := cm.getClient()
		if client.Healthy() && client.LatestBlockNumber() >= latestBlockNumber {
			return client.ETHClient, nil
		}
	}

	// Fallback: return the first healthy client.
	for i := 0; i < len(cm.clients); i++ {
		client := cm.getClient()
		if client.Healthy() {
			return client.ETHClient, nil
		}
	}

	return nil, errors.New("no healthy client")
}

// problem - client manager might have to manage multiple clients in different geographical locations
// users might need `preferred` clients, clients faster than the average
// how do we select these clients?
// we set a cutoff and we pick randomly
func (cm *ClientManager) GetPreferredClient() (ethclients.ETHClient, error) {
	// get latest known block
	latestBlockNumber := uint64(0)
	for _, c := range cm.clients {
		if c.LatestBlockNumber() > latestBlockNumber {
			latestBlockNumber = c.LatestBlockNumber()
		}
	}

	var healthyClients []*Client
	for _, client := range cm.clients {
		if client.Healthy() && client.LatestBlockNumber() >= latestBlockNumber {
			healthyClients = append(healthyClients, client)
		}
	}

	// if no healthy, default to any client
	if len(healthyClients) == 0 {
		return cm.GetClient()
	}

	// Sort healthy clients by latency (ascending)
	sort.Slice(healthyClients, func(i, j int) bool {
		return healthyClients[i].Latency() < healthyClients[j].Latency()
	})

	cutoff := healthyClients[0].Latency() * float64(cm.preferredClientMaxLatencyMultiplier)
	var preferredClients []*Client
	for _, client := range healthyClients {
		if client.Latency() <= cutoff {
			preferredClients = append(preferredClients, client)
		}
	}

	if len(preferredClients) == 0 {
		// If no clients under cutoff (extreme case), fallback to all healthy
		preferredClients = healthyClients
	}

	// Randomly select a preferred client
	randomIndex := rand.Intn(len(preferredClients))
	return preferredClients[randomIndex].ETHClient, nil
}

// GetClients returns all Ethereum clients.
func (cm *ClientManager) GetClients() []ethclients.ETHClient {
	c := []ethclients.ETHClient{}
	for _, client := range cm.clients {
		c = append(c, client.ETHClient)
	}
	return c
}

// Close gracefully cancels all subscriptions and closes all clients.
func (cm *ClientManager) Close() {
	if cm.closed.CompareAndSwap(false, true) {
		for _, client := range cm.clients {
			client.Close()
		}
		cm.logger.Info("all clients have been closed")
	}
}
