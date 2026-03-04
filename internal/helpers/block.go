package helpers

import (
	"context"
	"log/slog"

	"github.com/defistate/defistate/block"
	arbitrum_subscriber "github.com/defistate/defistate/block/subscriber/arbitrum"
	clientmanager "github.com/defistate/defistate/clients/eth-clients/client-manager"
	"github.com/ethereum/go-ethereum/core/types"
)

const (
	// DefaultNewBlockEventerBuffer is the buffer size for the main block event channel.
	DefaultNewBlockEventerBuffer = 100
)

// @todo Refactor to use Header Subscriptions for Cross-EVM Compatibility
// The current implementation relies on full Block subscriptions (*types.Block),
// which is fragile across different EVM chains (e.g., Arbitrum) due to
// custom transaction types.

// each powered by its own dedicated block subscriber.
func MakeBlockSubscriberGenerator(
	ctx context.Context,
	clientMgr *clientmanager.ClientManager,
	logger *slog.Logger,
	chainID uint64,
) (func(consumerName string) chan *types.Block, func() error) {
	chs := []chan *types.Block{}
	tags := []string{}

	start := func() error {
		eventer := make(chan *types.Block, DefaultNewBlockEventerBuffer)
		subscriberLogger := logger.With("component", "eth-block-subscriber")
		arbitrum_subscriber.NewBlockSubscriber(
			ctx,
			eventer,
			GetHealthyClientsForArbitrumSubscriber(clientMgr),
			&arbitrum_subscriber.SubscriberConfig{Logger: subscriberLogger},
		)

		return block.FanOutBlock(
			ctx,
			block.FanOutBlockConfig{
				Input:      eventer,
				Outputs:    chs,
				OutputTags: tags,
				Logger:     logger.With("component", "fan-out-block"),
			},
		)
	}

	return func(consumerName string) chan *types.Block {
		ch := make(chan *types.Block, DefaultNewBlockEventerBuffer)
		tags = append(tags, consumerName)
		chs = append(chs, ch)
		return ch
	}, start
}
