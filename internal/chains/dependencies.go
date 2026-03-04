package chains

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	"github.com/defistate/defistate/engine"
	token "github.com/defistate/defistate/protocols/erc20-token-system"
	poolregistry "github.com/defistate/defistate/protocols/pool-registry"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/prometheus/client_golang/prometheus"
)

// a structure containing all known protocol plugin interfaces
type ProtocolPlugins struct {
}

// Dependencies bundles the shared infrastructure required by all chains.
type Dependencies struct {
	GetClient                func() (ethclients.ETHClient, error)
	BlockSubscriberGenerator func(consumer string) chan *types.Block
	TokenSystem              *token.TokenSystem
	PoolSystem               *poolregistry.PoolSystem
	TokenPoolSystem          *poolregistry.TokenPoolSystem
	ProtocolPlugins          map[engine.ProtocolID]*ProtocolPlugins
	Registry                 *poolregistry.RegistryManager
	ErrorHandler             func(error)
	RootLogger               *slog.Logger
	PrometheusRegistry       prometheus.Registerer
}

// RegisterPlugin safely hooks a plugin into the dependencies.
// It handles map initialization to prevent nil map panics.
func (d *Dependencies) RegisterProtocolPlugins(protoID engine.ProtocolID, plugin *ProtocolPlugins) error {
	if d.ProtocolPlugins == nil {
		d.ProtocolPlugins = make(map[engine.ProtocolID]*ProtocolPlugins)
	}
	if _, exists := d.ProtocolPlugins[protoID]; exists {
		return errors.New("plugin already exists for protocol")
	}
	d.ProtocolPlugins[protoID] = plugin
	return nil
}

// Validate checks that all required dependencies are initialized.
// It returns a single error listing all missing fields, or nil if valid.
func (d *Dependencies) Validate() error {
	var missing []string

	if d.GetClient == nil {
		missing = append(missing, "GetClient")
	}
	if d.BlockSubscriberGenerator == nil {
		missing = append(missing, "BlockSubscriberGenerator")
	}
	if d.TokenSystem == nil {
		missing = append(missing, "TokenSystem")
	}
	if d.PoolSystem == nil {
		missing = append(missing, "PoolSystem")
	}
	if d.TokenPoolSystem == nil {
		missing = append(missing, "TokenPoolSystem")
	}
	if d.Registry == nil {
		missing = append(missing, "Registry")
	}
	if d.ErrorHandler == nil {
		missing = append(missing, "ErrorHandler")
	}
	if d.RootLogger == nil {
		missing = append(missing, "RootLogger")
	}
	if d.PrometheusRegistry == nil {
		missing = append(missing, "PrometheusRegistry")
	}

	if len(missing) > 0 {
		return fmt.Errorf("chains.Dependencies missing required fields: %s", strings.Join(missing, ", "))
	}

	return nil
}
