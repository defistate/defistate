package uniswapv3

import (
	"github.com/defistate/defistate/engine"
)

var UniswapV3ProtocolSchema engine.ProtocolSchema = "defistate/uniswap-v3@v1"

type UniswapV3Protocol struct {
	schema engine.ProtocolSchema
	meta   engine.ProtocolMeta
	*UniswapV3System
}

func NewUniswapV3Protocol(
	system *UniswapV3System,
) *UniswapV3Protocol {
	return &UniswapV3Protocol{
		schema: UniswapV3ProtocolSchema,
		meta: engine.ProtocolMeta{
			Name: engine.ProtocolName(system.systemName),
			Tags: []string{},
		},
		UniswapV3System: system,
	}
}

func (p *UniswapV3Protocol) View() (any, engine.ProtocolSchema, error) {
	// return system view, schema, and no error
	return p.UniswapV3System.View(), p.schema, nil
}

func (p *UniswapV3Protocol) Meta() engine.ProtocolMeta {
	return p.meta
}

func (p *UniswapV3Protocol) Schema() engine.ProtocolSchema {
	return p.schema
}
