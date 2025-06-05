//go:build ibmmq

package mqutils

import (
	"fmt"

	"github.com/ibm-messaging/mq-golang/v5/ibmmq"
)

// MQConnectionConfig contém os parâmetros de conexão para um Queue Manager MQ
type MQConnectionConfig struct {
	QueueManagerName    string
	ConnectionName      string
	Channel             string
	Username            string
	Password            string
	NonSharedConnection bool
}

// MQConnection encapsula uma conexão com um Queue Manager MQ
type MQConnection struct {
	QMgr        ibmmq.MQQueueManager
	Config      MQConnectionConfig
	IsConnected bool
}

// NewMQConnection cria uma nova instância de MQConnection
func NewMQConnection(config MQConnectionConfig) *MQConnection {
	return &MQConnection{
		Config:      config,
		IsConnected: false,
	}
}

// Connect estabelece uma conexão com o Queue Manager MQ
func (conn *MQConnection) Connect() error {
	cd := ibmmq.NewMQCD()
	cd.ChannelName = conn.Config.Channel
	cd.ConnectionName = conn.Config.ConnectionName

	cno := ibmmq.NewMQCNO()
	cno.ClientConn = cd

	if conn.Config.Username != "" {
		csp := ibmmq.NewMQCSP()
		csp.UserId = conn.Config.Username
		csp.Password = conn.Config.Password
		cno.SecurityParms = csp
	}

	var err error
	conn.QMgr, err = ibmmq.Connx(conn.Config.QueueManagerName, cno)
	if err != nil {
		return fmt.Errorf("falha ao conectar ao Queue Manager %s: %v", conn.Config.QueueManagerName, err)
	}

	conn.IsConnected = true
	return nil
}

// Disconnect fecha a conexão com o Queue Manager MQ
func (conn *MQConnection) Disconnect() error {
	if !conn.IsConnected {
		return nil
	}

	err := conn.QMgr.Disc()
	if err != nil {
		return fmt.Errorf("falha ao desconectar do Queue Manager: %v", err)
	}

	conn.IsConnected = false
	return nil
}

// OpenQueue abre uma fila MQ para leitura ou escrita
func (conn *MQConnection) OpenQueue(queueName string, forInput bool, nonShared bool) (ibmmq.MQObject, error) {
	if !conn.IsConnected {
		return ibmmq.MQObject{}, fmt.Errorf("não conectado ao Queue Manager")
	}

	var openOptions int32
	if forInput {
		openOptions = ibmmq.MQOO_INPUT_SHARED | ibmmq.MQOO_FAIL_IF_QUIESCING | ibmmq.MQOO_SAVE_ALL_CONTEXT
		if nonShared {
			openOptions = ibmmq.MQOO_INPUT_EXCLUSIVE | ibmmq.MQOO_FAIL_IF_QUIESCING | ibmmq.MQOO_SAVE_ALL_CONTEXT
		}
	} else {
		// Para manter todos os campos do MQMD, use apenas MQOO_SET_ALL_CONTEXT
		openOptions = ibmmq.MQOO_OUTPUT | ibmmq.MQOO_FAIL_IF_QUIESCING | ibmmq.MQOO_SET_ALL_CONTEXT
	}

	od := ibmmq.NewMQOD()
	od.ObjectName = queueName
	od.ObjectType = ibmmq.MQOT_Q

	queue, err := conn.QMgr.Open(od, openOptions)
	if err != nil {
		return ibmmq.MQObject{}, fmt.Errorf("falha ao abrir a fila %s: %v", queueName, err)
	}

	return queue, nil
}

// CloseQueue fecha uma fila MQ
func (conn *MQConnection) CloseQueue(queue ibmmq.MQObject) error {
	err := queue.Close(0)
	if err != nil {
		return fmt.Errorf("falha ao fechar a fila: %v", err)
	}
	return nil
}

// GetMessage obtém uma mensagem de uma fila MQ
func (conn *MQConnection) GetMessage(queue ibmmq.MQObject, bufferSize int, waitInterval int) ([]byte, *ibmmq.MQMD, error) {
	md := ibmmq.NewMQMD()

	gmo := ibmmq.NewMQGMO()
	gmo.Options = ibmmq.MQGMO_WAIT | ibmmq.MQGMO_FAIL_IF_QUIESCING
	if waitInterval > 0 {
		gmo.Options |= ibmmq.MQGMO_SYNCPOINT
	} else {
		gmo.Options |= ibmmq.MQGMO_NO_SYNCPOINT
	}
	gmo.WaitInterval = 5 * 1000

	buffer := make([]byte, bufferSize)

	datalen, err := queue.Get(md, gmo, buffer)
	if err != nil {
		mqrc := err.(*ibmmq.MQReturn).MQRC
		if mqrc == ibmmq.MQRC_NO_MSG_AVAILABLE {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("falha ao obter mensagem: %v", err)
	}

	return buffer[:datalen], md, nil
}

// PutMessage coloca uma mensagem em uma fila MQ, preservando o contexto da mensagem original
// contextType: "pass" para MQPMO_PASS_ALL_CONTEXT, "set" para MQPMO_SET_ALL_CONTEXT, "none" para MQPMO_NO_CONTEXT
func (conn *MQConnection) PutMessage(queue ibmmq.MQObject, data []byte, md *ibmmq.MQMD, commitInterval int, contextType string) error {
	pmo := ibmmq.NewMQPMO()
	if commitInterval > 0 {
		pmo.Options |= ibmmq.MQPMO_SYNCPOINT
	} else {
		pmo.Options |= ibmmq.MQPMO_NO_SYNCPOINT
	}

	switch contextType {
	case "pass":
		pmo.Options |= ibmmq.MQPMO_PASS_ALL_CONTEXT
	case "set":
		pmo.Options |= ibmmq.MQPMO_SET_ALL_CONTEXT
	default:
		pmo.Options |= ibmmq.MQPMO_NO_CONTEXT
	}

	err := queue.Put(md, pmo, data)
	if err != nil {
		return fmt.Errorf("falha ao colocar mensagem: %v", err)
	}

	return nil
}

// Commit realiza um commit da transação atual
func (conn *MQConnection) Commit() error {
	if !conn.IsConnected {
		return fmt.Errorf("não conectado ao Queue Manager")
	}

	err := conn.QMgr.Cmit()
	if err != nil {
		return fmt.Errorf("falha ao realizar commit: %v", err)
	}

	return nil
}

// Backout realiza um backout da transação atual
func (conn *MQConnection) Backout() error {
	if !conn.IsConnected {
		return fmt.Errorf("não conectado ao Queue Manager")
	}

	err := conn.QMgr.Back()
	if err != nil {
		return fmt.Errorf("falha ao realizar backout: %v", err)
	}

	return nil
}

// NewCleanMQMD cria um novo MQMD limpo, copiando apenas campos essenciais de outro MQMD
func NewCleanMQMD(src *ibmmq.MQMD) *ibmmq.MQMD {
	if src == nil {
		return ibmmq.NewMQMD()
	}
	md := ibmmq.NewMQMD()
	md.Format = src.Format
	md.Priority = src.Priority
	md.Persistence = src.Persistence
	md.CorrelId = src.CorrelId
	md.MsgId = src.MsgId
	md.UserIdentifier = src.UserIdentifier
	md.PutApplName = src.PutApplName
	md.PutDate = src.PutDate
	md.PutTime = src.PutTime
	return md
}
