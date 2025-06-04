//go:build !ibmmq

package mqutils

import "errors"

// MQConnectionConfig holds MQ connection parameters.
type MQConnectionConfig struct {
	QueueManagerName    string
	ConnectionName      string
	Channel             string
	Username            string
	Password            string
	NonSharedConnection bool
}

// MQConnection is a stub implementation used when the ibmmq C libraries are not available.
type MQConnection struct {
	Config      MQConnectionConfig
	IsConnected bool
}

func NewMQConnection(config MQConnectionConfig) *MQConnection {
	return &MQConnection{Config: config}
}

func (c *MQConnection) Connect() error {
	c.IsConnected = true
	return nil
}

func (c *MQConnection) Disconnect() error {
	c.IsConnected = false
	return nil
}

func (c *MQConnection) OpenQueue(queueName string, forInput bool, nonShared bool) (struct{}, error) {
	if !c.IsConnected {
		return struct{}{}, errors.New("not connected")
	}
	return struct{}{}, nil
}

func (c *MQConnection) CloseQueue(queue struct{}) error { return nil }

func (c *MQConnection) GetMessage(queue struct{}, bufferSize int, waitInterval int) ([]byte, interface{}, error) {
	return nil, nil, nil
}

func (c *MQConnection) PutMessage(queue struct{}, data []byte, md interface{}, commitInterval int) error {
	return nil
}

func (c *MQConnection) Commit() error { return nil }

func (c *MQConnection) Backout() error { return nil }
