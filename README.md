# MQ Forwarder

Aplicação escrita em Go para transferir mensagens entre filas IBM MQ. Ela expõe uma API REST usando o framework Gin e está instrumentada automaticamente com o Datadog Orchestrion.

Cada instância de worker abre conexões próprias com as filas de origem e destino, permitindo o processamento totalmente paralelo em máquinas multi-core.

A lógica de transferência está no pacote `internal/transfer`, que possui uma implementação simplificada para facilitar testes. A integração real com o MQ só é ativada quando o projeto é compilado com as bibliotecas C do IBM MQ (tag de build `ibmmq`).

Para instruções de instalação e exemplos de uso, consulte o arquivo [GETTING_STARTED.md](GETTING_STARTED.md).

## Documentação da API de Transferência MQ

Esta seção descreve os principais endpoints e parâmetros disponíveis para controlar a transferência de mensagens.

### Visão Geral

A API permite transferir mensagens entre filas IBM MQ, preservando o contexto completo das mensagens. As operações são assíncronas e podem ser monitoradas via endpoints de status.

### Iniciar Transferência

`POST /api/v1/transfer`

Payload de exemplo:
```json
{
  "source": {
    "queueManagerName": "QM1",
    "connectionName": "localhost(1414)",
    "channel": "SYSTEM.DEF.SVRCONN",
    "username": "mquser",
    "password": "mqpassword"
  },
  "sourceQueue": "SOURCE.QUEUE",
  "destination": {
    "queueManagerName": "QM2",
    "connectionName": "remotehost(1414)",
    "channel": "SYSTEM.DEF.SVRCONN",
    "username": "mquser",
    "password": "mqpassword"
  },
  "destinationQueue": "DEST.QUEUE",
  "bufferSize": 1048576,
  "nonSharedConnection": false,
  "commitInterval": 10
}
```

Resposta de exemplo:
```json
{
  "requestId": "550e8400-e29b-41d4-a716-446655440000",
  "status": "in_progress"
}
```

### Obter Status da Transferência

`GET /api/v1/transfer/{requestId}`

Retorna as informações atuais de uma transferência em andamento.

### Listar Todas as Transferências

`GET /api/v1/transfers`

Lista as transferências iniciadas e seus respectivos status.

### Cancelar Transferência

`POST /api/v1/transfer/{requestId}/cancel`

Cancela uma transferência em andamento.

### Health Check

`GET /api/v1/health`

Retorna o status de saúde e a versão atual da aplicação.

### Parâmetros de Configuração

- **bufferSize**: tamanho do buffer para operação `MQGET` (padrão 1MB).
- **nonSharedConnection**: abre a fila de origem em modo exclusivo quando `true`.
- **commitInterval**: define o número de mensagens processadas antes do commit. Caso omisso, o valor padrão de 10 é utilizado.

### Variáveis de Ambiente

- **WORKER_COUNT**: define o número de goroutines/threads de transferência. Cada worker mantém suas próprias conexões de origem e destino, permitindo aproveitar todos os núcleos disponíveis.
- **BATCH_SIZE**: tamanho do lote de mensagens antes do commit quando `commitInterval` não é especificado na requisição.
- **BUFFER_SIZE**: define o tamanho do buffer utilizado para ler mensagens quando `bufferSize` não é informado na requisição.

### Observabilidade com Datadog

A aplicação utiliza o [Datadog Orchestrion](https://docs.datadoghq.com/tracing/trace_collection/automatic_instrumentation/dd_libraries/go/?tab=compiletimeinstrumentation) para adicionar instrumentação automática e enviar traces ao Datadog.

#### Instalação e uso

1. Instale o Orchestrion:

```bash
go install github.com/DataDog/orchestrion@latest
```

2. (Opcional) atualize as integrações detectadas:

```bash
orchestrion pin
```

3. Execute ou compile a aplicação usando o wrapper do Orchestrion:

```bash
orchestrion go run .
# ou
orchestrion go build
```

A documentação OpenAPI gerada automaticamente pode ser acessada em `/swagger/index.html` quando o servidor está em execução.

