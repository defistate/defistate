package poolregistry

import "github.com/defistate/defistate/engine"

var TokenPoolProtocolSchema engine.ProtocolSchema = "defistate/token-pool-registry@v1"

type TokenPoolProtocol struct {
	schema engine.ProtocolSchema
	meta   engine.ProtocolMeta
	*TokenPoolSystem
}

func NewTokenPoolProtocol(
	tokenPoolSystem *TokenPoolSystem,
) *TokenPoolProtocol {
	return &TokenPoolProtocol{
		schema: TokenPoolProtocolSchema,
		meta: engine.ProtocolMeta{
			Name: "token pool graph",
			Tags: []string{"graph"},
		},
		TokenPoolSystem: tokenPoolSystem,
	}
}

func (p *TokenPoolProtocol) View() (any, engine.ProtocolSchema, error) {
	// return system view, schema, and no error
	return p.TokenPoolSystem.View(), p.schema, nil
}

func (p *TokenPoolProtocol) Meta() engine.ProtocolMeta {
	return p.meta
}

func (p *TokenPoolProtocol) Schema() engine.ProtocolSchema {
	return p.schema
}
