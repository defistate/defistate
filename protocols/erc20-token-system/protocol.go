package token

import "github.com/defistate/defistate/engine"

var TokenProtocolSchema engine.ProtocolSchema = "defistate/token-registry@v1"

type TokenProtocol struct {
	schema engine.ProtocolSchema
	meta   engine.ProtocolMeta
	*TokenSystem
}

func NewTokenProtocol(
	tokenSystem *TokenSystem,
) *TokenProtocol {
	return &TokenProtocol{
		schema: TokenProtocolSchema,
		meta: engine.ProtocolMeta{
			Name: "token system",
			Tags: []string{"erc20"},
		},
		TokenSystem: tokenSystem,
	}
}

func (p *TokenProtocol) View() (any, engine.ProtocolSchema, error) {
	// return system view, schema, and no error
	return p.TokenSystem.View(), p.schema, nil
}

func (p *TokenProtocol) Meta() engine.ProtocolMeta {
	return p.meta
}

func (p *TokenProtocol) Schema() engine.ProtocolSchema {
	return p.schema
}
