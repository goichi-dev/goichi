package mqtt

import (
	"context"
	"fmt"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"log"
	"net"
	"sync"
	"time"
)

type MQTTServer struct {
	config    MQTTConfig
	client    mqtt.Client
	isRunning bool
	mu        sync.RWMutex
}

func NewMQTTServer(config MQTTConfig) *MQTTServer {
	config.SetDefaults()

	opts := mqtt.NewClientOptions()
	brokerAddr := fmt.Sprintf("tcp://%s:%d", config.Address, config.Port)
	opts.AddBroker(brokerAddr)
	opts.SetClientID(fmt.Sprintf("goichi-server-%d", time.Now().UnixNano()))
	opts.SetKeepAlive(time.Duration(config.KeepAlive) * time.Second)
	opts.SetAutoReconnect(true)

	client := mqtt.NewClient(opts)

	return &MQTTServer{
		config: config,
		client: client,
	}
}

func (ms *MQTTServer) GetInfo() string {
	return fmt.Sprintf("MQTT Client connected to %s:%d", ms.config.Address, ms.config.Port)
}

func (ms *MQTTServer) GetName() string {
	return "mqtt-server"
}

func (ms *MQTTServer) GetPaths() []string {
	return nil
}

func (ms *MQTTServer) SetListener(ln net.Listener) {
	// Not used for MQTT client
}

func (ms *MQTTServer) Start(ctx context.Context) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if ms.isRunning {
		return nil
	}

	if !ms.config.Enabled {
		log.Printf("[MQTT Server] Disabled, skipping start")
		return nil
	}

	if token := ms.client.Connect(); token.Wait() && token.Error() != nil {
		return token.Error()
	}

	ms.isRunning = true
	log.Printf("[MQTT Server] Connected to broker at %s:%d", ms.config.Address, ms.config.Port)
	return nil
}

func (ms *MQTTServer) Stop(ctx context.Context) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if !ms.isRunning {
		return nil
	}

	ms.client.Disconnect(250)
	ms.isRunning = false
	log.Printf("[MQTT Server] Disconnected")
	return nil
}

func (ms *MQTTServer) IsRunning() bool {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return ms.isRunning
}

func (ms *MQTTServer) Publish(topic string, qos byte, retained bool, payload interface{}) error {
	token := ms.client.Publish(topic, qos, retained, payload)
	token.Wait()
	return token.Error()
}

func (ms *MQTTServer) Subscribe(topic string, qos byte, callback mqtt.MessageHandler) error {
	token := ms.client.Subscribe(topic, qos, callback)
	token.Wait()
	return token.Error()
}

func (ms *MQTTServer) Unsubscribe(topic string) error {
	token := ms.client.Unsubscribe(topic)
	token.Wait()
	return token.Error()
}
