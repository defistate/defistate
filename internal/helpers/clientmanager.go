package helpers

import (
	arbitrum_subscriber "github.com/defistate/defistate/block/subscriber/arbitrum"
	subscriber "github.com/defistate/defistate/block/subscriber/ethereum"

	clientmanager "github.com/defistate/defistate/clients/eth-clients/client-manager"
)

// GetHealthyClientsForSubscriber returns a function that provides a slice of unique, healthy clients
// for the block subscriber to use.
func GetHealthyClientsForSubscriber(clientManager *clientmanager.ClientManager) func() []subscriber.ETHClient {
	return func() []subscriber.ETHClient {
		clients := []subscriber.ETHClient{}
		known := map[subscriber.ETHClient]struct{}{}
		for {
			client, err := clientManager.GetClient()
			if err != nil {
				break
			}
			if _, exists := known[client]; exists {
				break
			}
			known[client] = struct{}{}
		}
		for c := range known {
			clients = append(clients, c)
		}
		return clients
	}
}

// GetHealthyClientsForArbitrumSubscriber returns a function that provides a slice of unique, healthy clients
// for the block subscriber to use.
func GetHealthyClientsForArbitrumSubscriber(clientManager *clientmanager.ClientManager) func() []arbitrum_subscriber.ETHClient {
	return func() []arbitrum_subscriber.ETHClient {
		clients := []arbitrum_subscriber.ETHClient{}
		known := map[arbitrum_subscriber.ETHClient]struct{}{}
		for {
			client, err := clientManager.GetClient()
			if err != nil {
				break
			}
			if _, exists := known[client]; exists {
				break
			}
			known[client] = struct{}{}
		}
		for c := range known {
			clients = append(clients, c)
		}
		return clients
	}
}
