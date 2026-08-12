package grpc

import "github.com/goichi-dev/goichi/protocol"

type GRPCConfig struct {
	protocol.ProtocolConfig
	MaxConcurrentStreams uint32 // default 100
	MaxRecvMsgSize       int    // default 4MB
	MaxSendMsgSize       int    // default 4MB
	KeepaliveTime        int    // seconds, default 2 hours
}

func (c *GRPCConfig) SetDefaults() {
	if c.Address == "" {
		c.Address = "0.0.0.0"
	}
	if c.Port == 0 {
		c.Port = 8082
	}
	if c.MaxConcurrentStreams == 0 {
		c.MaxConcurrentStreams = 100
	}
	if c.MaxRecvMsgSize == 0 {
		c.MaxRecvMsgSize = 4 * 1024 * 1024 // 4MB
	}
	if c.MaxSendMsgSize == 0 {
		c.MaxSendMsgSize = 4 * 1024 * 1024 // 4MB
	}
	if c.KeepaliveTime == 0 {
		c.KeepaliveTime = 7200 // 2 hours
	}
}
