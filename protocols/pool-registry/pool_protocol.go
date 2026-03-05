package poolregistry

import (
	"github.com/defistate/defistate/engine"
)

var PoolProtocolSchema engine.ProtocolSchema = "defistate/pool-registry@v1"

type PoolProtocol struct {
	schema engine.ProtocolSchema
	meta   engine.ProtocolMeta
	*PoolSystem
}

func NewPoolProtocol(
	poolSystem *PoolSystem,
) *PoolProtocol {
	return &PoolProtocol{
		schema: PoolProtocolSchema,
		meta: engine.ProtocolMeta{
			Name: "pool protocol",
			Tags: []string{""},
		},
		PoolSystem: poolSystem,
	}
}

func (p *PoolProtocol) View() (any, engine.ProtocolSchema, error) {
	// return system view, schema, and no error
	return p.PoolSystem.View(), p.schema, nil
}

func (p *PoolProtocol) Meta() engine.ProtocolMeta {
	return p.meta
}

func (p *PoolProtocol) Schema() engine.ProtocolSchema {
	return p.schema
}
