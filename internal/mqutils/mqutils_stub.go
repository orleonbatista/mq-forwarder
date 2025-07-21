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

// Testing controls
var (
	ReturnNilMessage bool
	FailConnectCall  int
	connectCalls     int
	FailOpenCall     int
	openCalls        int
	FailPut          bool
	FailCommit       bool
	FailGet          bool
	Messages         []bool
	msgIndex         int
)

func ResetTestState() {
	ReturnNilMessage = false
	FailConnectCall = 0
	connectCalls = 0
	FailOpenCall = 0
	openCalls = 0
	FailPut = false
	FailCommit = false
	FailGet = false
	Messages = nil
	msgIndex = 0
}

func NewMQConnection(config MQConnectionConfig) *MQConnection {
	return &MQConnection{Config: config}
}

func (c *MQConnection) Connect() error {
	connectCalls++
	if FailConnectCall > 0 && connectCalls == FailConnectCall {
		return errors.New("connect fail")
	}
	c.IsConnected = true
	return nil
}

func (c *MQConnection) Disconnect() error {
	c.IsConnected = false
	return nil
}

func (c *MQConnection) OpenQueue(queueName string, forInput bool, nonShared bool) (struct{}, error) {
	openCalls++
	if FailOpenCall > 0 && openCalls == FailOpenCall {
		return struct{}{}, errors.New("open fail")
	}
	if !c.IsConnected {
		return struct{}{}, errors.New("not connected")
	}
	return struct{}{}, nil
}

func (c *MQConnection) CloseQueue(queue struct{}) error { return nil }

func (c *MQConnection) GetMessage(queue struct{}, buffer []byte) ([]byte, interface{}, error) {
	if FailGet {
		FailGet = false
		return nil, nil, errors.New("get fail")
	}
	if ReturnNilMessage {
		return nil, nil, nil
	}
	if len(Messages) > 0 {
		if msgIndex < len(Messages) {
			ret := Messages[msgIndex]
			msgIndex++
			if !ret {
				return nil, nil, nil
			}
		} else {
			return nil, nil, nil
		}
	}
	if len(buffer) == 0 {
		buffer = make([]byte, 1)
	}
	return buffer[:0], nil, nil
}

func (c *MQConnection) PutMessage(queue struct{}, data []byte, md interface{}, contextType string) error {
	if FailPut {
		FailPut = false
		return errors.New("put fail")
	}
	return nil
}

func (c *MQConnection) Commit() error {
	if FailCommit {
		FailCommit = false
		return errors.New("commit fail")
	}
	return nil
}

func (c *MQConnection) Backout() error { return nil }
