package graphql

import "github.com/goichi-dev/goichi/protocol"

type GraphQLConfig struct {
	protocol.ProtocolConfig
	QueryPath      string // default "/graphql"
	PlaygroundPath string // default "/graphql/playground"
	MaxQueryDepth  int    // default 10
	Timeout        int    // seconds, default 30

	// MaxComplexity caps the total query complexity to mitigate resource-
	// exhaustion via deeply nested / expensive queries. Defaults to
	// MaxQueryDepth * 100 when unset.
	MaxComplexity int

	// EnablePlayground exposes the GraphQL playground UI. Disabled by default
	// because it also implies schema introspection — keep it off in production.
	EnablePlayground bool

	// EnableIntrospection allows __schema / __type introspection queries.
	// Disabled by default; enabling the playground implies introspection.
	EnableIntrospection bool

	// RequireAuth rejects queries without a valid JWT. Implied automatically
	// when a unified auth secret is configured.
	RequireAuth bool
}

func (c *GraphQLConfig) SetDefaults() {
	if c.Address == "" {
		c.Address = "0.0.0.0"
	}
	if c.Port == 0 {
		c.Port = 8081
	}
	if c.QueryPath == "" {
		c.QueryPath = "/graphql"
	}
	if c.PlaygroundPath == "" {
		c.PlaygroundPath = "/graphql/playground"
	}
	if c.MaxQueryDepth == 0 {
		c.MaxQueryDepth = 10
	}
	if c.Timeout == 0 {
		c.Timeout = 30
	}
	if c.MaxComplexity == 0 {
		c.MaxComplexity = c.MaxQueryDepth * 100
	}
	// The playground requires introspection to function.
	if c.EnablePlayground {
		c.EnableIntrospection = true
	}
}
