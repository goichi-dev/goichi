package mqtt

import "github.com/goichi-dev/goichi/protocol"

type MQTTConfig struct {
	protocol.ProtocolConfig
	BrokerPath        string // default "/mqtt"
	MaxMessageSize    int    // bytes, default 256KB
	KeepAlive         int    // seconds, default 60
	MaxQueuedMessages int    // default 1000
	PersistencePath   string // path for persistence storage
	AuthEnabled       bool
	AllowAnonymous    bool // default true
	MaxConnections    int  // default 5000
}

func (c *MQTTConfig) SetDefaults() {
	if c.Address == "" {
		c.Address = "0.0.0.0"
	}
	if c.Port == 0 {
		c.Port = 1883
	}
	if c.BrokerPath == "" {
		c.BrokerPath = "/mqtt"
	}
	if c.MaxMessageSize == 0 {
		c.MaxMessageSize = 256 * 1024 // 256KB
	}
	if c.KeepAlive == 0 {
		c.KeepAlive = 60
	}
	if c.MaxQueuedMessages == 0 {
		c.MaxQueuedMessages = 1000
	}
	if c.MaxConnections == 0 {
		c.MaxConnections = 5000
	}
	if !c.AuthEnabled {
		c.AllowAnonymous = true
	}
}
