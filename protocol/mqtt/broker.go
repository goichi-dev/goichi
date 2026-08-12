package mqtt

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"github.com/goichi-dev/goichi/middleware"
	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/mochi-mqtt/server/v2/packets"
	"log"
	"log/slog"
	"net"
	"sync"
)

type injectedListener struct {
	id string
	ln net.Listener
}

func (l *injectedListener) ID() string {
	return l.id
}

func (l *injectedListener) Address() string {
	return l.ln.Addr().String()
}

func (l *injectedListener) Protocol() string {
	return "tcp"
}

func (l *injectedListener) Init(logger *slog.Logger) error {
	return nil
}

func (l *injectedListener) Serve(onConnect listeners.EstablishFn) {
	for {
		conn, err := l.ln.Accept()
		if err != nil {
			return
		}
		onConnect(l.id, conn)
	}
}

func (l *injectedListener) Close(onClose listeners.CloseFn) {
	l.ln.Close()
	onClose(l.id)
}

// authHook is the single source of truth for MQTT connection authentication.
// It reads the broker's live auth state so that precedence is unambiguous:
//   - if a JWT secret is configured, the CONNECT password MUST be a valid JWT;
//   - otherwise anonymous connections are allowed only when AllowAnonymous is set.
//
// This replaces the previous design where a permissive AllowHook could win over
// the JWT hook and silently bypass authentication.
type authHook struct {
	mqtt.HookBase
	broker *MQTTBroker
}

func (h *authHook) ID() string {
	return "goichi-auth"
}

func (h *authHook) Provides(b byte) bool {
	return bytes.Contains([]byte{
		mqtt.OnConnectAuthenticate,
		mqtt.OnACLCheck,
	}, []byte{b})
}

func (h *authHook) OnConnectAuthenticate(cl *mqtt.Client, pk packets.Packet) bool {
	secret, allowAnon := h.broker.authState()

	if secret == "" {
		return allowAnon
	}

	// The 'Password' field in the CONNECT packet carries the JWT.
	token := string(pk.Connect.Password)
	_, errMsg := middleware.ValidateJWT(token, secret)
	return errMsg == ""
}

// OnACLCheck authorizes publish/subscribe for already-authenticated clients.
func (h *authHook) OnACLCheck(cl *mqtt.Client, topic string, write bool) bool {
	return true
}

type Message struct {
	Topic    string
	QoS      int
	Retain   bool
	Payload  []byte
	ClientID string
}

type MQTTBroker struct {
	config    MQTTConfig
	server    *mqtt.Server
	isRunning bool
	mu        sync.RWMutex
	listener  net.Listener

	authSecret string
	authLookup string

	// subs maps a topic filter to the inline subscription id held by the broker
	// itself, so a filter can be replaced or removed later.
	subs   map[string]int
	subSeq int
}

func (mb *MQTTBroker) SetListener(ln net.Listener) {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	mb.listener = ln
}

// SetAuthSecret updates the live JWT secret. The auth hook reads this value on
// every CONNECT, so updates take effect immediately and cannot be bypassed.
func (mb *MQTTBroker) SetAuthSecret(secret string) {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	mb.authSecret = secret
}

func (mb *MQTTBroker) SetAuthLookup(lookup string) {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	mb.authLookup = lookup
}

// authState returns the current secret and whether anonymous connections are
// permitted, read under the broker lock.
func (mb *MQTTBroker) authState() (secret string, allowAnonymous bool) {
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	return mb.authSecret, mb.config.AllowAnonymous && !mb.config.AuthEnabled
}

func NewMQTTBroker(config MQTTConfig) *MQTTBroker {
	config.SetDefaults()

	server := mqtt.New(&mqtt.Options{
		InlineClient: true, // Allows us to use Publish/Subscribe directly on the server object
	})

	mb := &MQTTBroker{
		config: config,
		server: server,
	}

	// Exactly one auth hook governs all connections. It decides at CONNECT time
	// based on the live auth state (JWT secret vs. AllowAnonymous), so there is
	// no permissive hook that can override JWT validation.
	_ = server.AddHook(&authHook{broker: mb}, nil)

	return mb
}

func (mb *MQTTBroker) GetInfo() string {
	return fmt.Sprintf("MQTT Broker (Port %d)", mb.config.Port)
}

func (mb *MQTTBroker) GetName() string {
	return "mqtt"
}

func (mb *MQTTBroker) GetPaths() []string {
	return nil
}

func (mb *MQTTBroker) Start(ctx context.Context) error {
	mb.mu.Lock()
	if mb.isRunning {
		mb.mu.Unlock()
		return nil
	}
	mb.mu.Unlock()

	if !mb.config.Enabled {
		log.Printf("[MQTT] Disabled, skipping start")
		return nil
	}

	var listener net.Listener
	var err error
	var addr string

	mb.mu.RLock()
	listener = mb.listener
	mb.mu.RUnlock()

	var tlsConfig *tls.Config
	if mb.config.TLSEnabled() {
		cert, cerr := tls.LoadX509KeyPair(mb.config.TLSCertFile, mb.config.TLSKeyFile)
		if cerr != nil {
			return fmt.Errorf("mqtt tls: %w", cerr)
		}
		tlsConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}
	}

	if listener == nil {
		addr = fmt.Sprintf("%s:%d", mb.config.Address, mb.config.Port)
		tcp := listeners.NewTCP(listeners.Config{
			ID:        "t1",
			Address:   addr,
			TLSConfig: tlsConfig,
		})
		err = mb.server.AddListener(tcp)
		if err != nil {
			return err
		}
	} else {
		addr = "injected listener"
		if tlsConfig != nil {
			listener = tls.NewListener(listener, tlsConfig)
		}
		err = mb.server.AddListener(&injectedListener{
			id: "t1",
			ln: listener,
		})
		if err != nil {
			return err
		}
	}

	go func() {
		err := mb.server.Serve()
		if err != nil {
			log.Printf("[MQTT] Server error: %v", err)
		}
	}()

	mb.mu.Lock()
	mb.isRunning = true
	mb.mu.Unlock()

	log.Printf("[MQTT] Broker started on %s", addr)
	return nil
}

func (mb *MQTTBroker) Stop(ctx context.Context) error {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	if !mb.isRunning {
		return nil
	}

	err := mb.server.Close()
	mb.isRunning = false
	log.Printf("[MQTT] Broker stopped")
	return err
}

func (mb *MQTTBroker) IsRunning() bool {
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	return mb.isRunning
}

func (mb *MQTTBroker) Publish(ctx context.Context, topic string, payload []byte, qos byte, retain bool) error {
	return mb.server.Publish(topic, payload, retain, qos)
}

// Subscribe registers a server-side handler for every message published to a
// topic filter, including messages the broker publishes itself. The filter may
// contain the usual MQTT wildcards ("+" and "#").
//
// Subscribing twice to the same filter replaces the previous handler.
func (mb *MQTTBroker) Subscribe(topic string, handler func(topic string, payload []byte)) error {
	if handler == nil {
		return fmt.Errorf("mqtt: subscribe %q: handler is nil", topic)
	}

	mb.mu.Lock()
	if mb.subs == nil {
		mb.subs = make(map[string]int)
	}
	id, exists := mb.subs[topic]
	if !exists {
		mb.subSeq++
		id = mb.subSeq
		mb.subs[topic] = id
	}
	mb.mu.Unlock()

	if exists {
		_ = mb.server.Unsubscribe(topic, id)
	}

	return mb.server.Subscribe(topic, id, func(cl *mqtt.Client, sub packets.Subscription, pk packets.Packet) {
		handler(pk.TopicName, pk.Payload)
	})
}

// Unsubscribe removes a handler previously registered with Subscribe.
func (mb *MQTTBroker) Unsubscribe(topic string) error {
	mb.mu.Lock()
	id, ok := mb.subs[topic]
	delete(mb.subs, topic)
	mb.mu.Unlock()

	if !ok {
		return nil
	}
	return mb.server.Unsubscribe(topic, id)
}

func (mb *MQTTBroker) GetClientCount() int {
	return int(mb.server.Info.ClientsConnected)
}
