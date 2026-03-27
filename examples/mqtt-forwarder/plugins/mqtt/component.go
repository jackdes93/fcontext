package mqtt

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sync"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/jackdes93/fcontext"
)

type MQTTComponent struct {
	id       string
	log      fcontext.Logger
	client   mqtt.Client
	cfg      Config
	topics   map[string]byte // topic -> QoS
	mu       sync.RWMutex
	onMessage MessageHandler
}

type Config struct {
	Broker   string
	Port     int
	Username string
	Password string
	ClientID string
}

// MessageHandler được gọi khi nhận message từ MQTT
type MessageHandler func(topic string, payload []byte)

func NewComponent(id string) *MQTTComponent {
	return &MQTTComponent{
		id:    id,
		topics: make(map[string]byte),
	}
}

func (c *MQTTComponent) WithTopics(topics map[string]byte) *MQTTComponent {
	c.mu.Lock()
	defer c.mu.Unlock()
	for topic, qos := range topics {
		c.topics[topic] = qos
	}
	return c
}

func (c *MQTTComponent) WithMessageHandler(h MessageHandler) *MQTTComponent {
	c.onMessage = h
	return c
}

func (c *MQTTComponent) ID() string { return c.id }

func (c *MQTTComponent) InitFlags() {
	c.cfg.Broker = os.Getenv("MQTT_BROKER")
	if c.cfg.Broker == "" {
		c.cfg.Broker = "localhost"
	}
	flag.IntVar(&c.cfg.Port, "mqtt-port", 1883, "MQTT broker port")
	flag.StringVar(&c.cfg.Broker, "mqtt-broker", c.cfg.Broker, "MQTT broker address")
	flag.StringVar(&c.cfg.Username, "mqtt-user", "", "MQTT username")
	flag.StringVar(&c.cfg.Password, "mqtt-pass", "", "MQTT password")
	flag.StringVar(&c.cfg.ClientID, "mqtt-client-id", "fcontext-mqtt-client", "MQTT client ID")
}

func (c *MQTTComponent) Order() int { return 30 } // khởi động trước worker

func (c *MQTTComponent) Activate(ctx context.Context, sv fcontext.ServiceContext) error {
	c.log = sv.Logger(c.ID())

	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%d", c.cfg.Broker, c.cfg.Port))
	opts.SetClientID(c.cfg.ClientID)
	
	if c.cfg.Username != "" {
		opts.SetUsername(c.cfg.Username)
	}
	if c.cfg.Password != "" {
		opts.SetPassword(c.cfg.Password)
	}

	// Callback khi nhận message
	opts.SetDefaultPublishHandler(func(_ mqtt.Client, msg mqtt.Message) {
		c.log.Info("received message", map[string]interface{}{
			"topic": msg.Topic(),
			"size":  len(msg.Payload()),
		})
		if c.onMessage != nil {
			c.onMessage(msg.Topic(), msg.Payload())
		}
	})

	opts.SetOnConnectHandler(func(client mqtt.Client) {
		c.log.Info("mqtt connected")
		// Subscribe to topics
		c.mu.RLock()
		topics := c.topics
		c.mu.RUnlock()

		for topic, qos := range topics {
			if token := client.Subscribe(topic, qos, nil); token.Wait() && token.Error() != nil {
				c.log.Error("subscribe failed", "topic", topic, "error", token.Error())
			} else {
				c.log.Info("subscribed", "topic", topic)
			}
		}
	})

	opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		c.log.Error("mqtt connection lost", "error", err)
	})

	c.client = mqtt.NewClient(opts)
	if token := c.client.Connect(); token.Wait() && token.Error() != nil {
		return fmt.Errorf("mqtt connect failed: %w", token.Error())
	}

	c.log.Info("mqtt component started", "broker", fmt.Sprintf("%s:%d", c.cfg.Broker, c.cfg.Port))
	return nil
}

func (c *MQTTComponent) Stop(ctx context.Context) error {
	if c.client != nil && c.client.IsConnected() {
		c.client.Disconnect(250)
	}
	return nil
}

// Publish method để gửi message
func (c *MQTTComponent) Publish(topic string, payload []byte, qos byte) error {
	if !c.client.IsConnected() {
		return fmt.Errorf("mqtt not connected")
	}
	token := c.client.Publish(topic, qos, false, payload)
	if token.Wait() && token.Error() != nil {
		return token.Error()
	}
	return nil
}
