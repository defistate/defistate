package uniswapv2

import "github.com/defistate/defistate/engine"

var UniswapV2ProtocolSchema engine.ProtocolSchema = "defistate/uniswap-v2@v1"

type UniswapV2Protocol struct {
	schema engine.ProtocolSchema
	meta   engine.ProtocolMeta
	*UniswapV2System
}

func NewUniswapV2Protocol(
	system *UniswapV2System,
) *UniswapV2Protocol {
	return &UniswapV2Protocol{
		schema: UniswapV2ProtocolSchema,
		meta: engine.ProtocolMeta{
			Name: engine.ProtocolName(system.systemName),
			Tags: []string{},
		},
		UniswapV2System: system,
	}
}

func (p *UniswapV2Protocol) View() (any, engine.ProtocolSchema, error) {
	// return system view, schema, and no error
	return p.UniswapV2System.View(), p.schema, nil
}

func (p *UniswapV2Protocol) Meta() engine.ProtocolMeta {
	return p.meta
}

func (p *UniswapV2Protocol) Schema() engine.ProtocolSchema {
	return p.schema
}
