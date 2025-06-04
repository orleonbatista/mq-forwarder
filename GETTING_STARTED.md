# Getting Started

Este guia descreve como compilar e executar o MQ Forwarder.

## Pré-requisitos

- Go 1.23 ou superior
- (Opcional) Bibliotecas C do IBM MQ caso deseje executar a transferência real de mensagens

## Compilação

### Sem IBM MQ (modo de simulação)

```bash
go run main.go
```

### Com suporte a IBM MQ

Instale as bibliotecas C do IBM MQ e compile com a tag de build `ibmmq`:

```bash
go run -tags ibmmq main.go
```

## Variáveis de ambiente

- `ENV`: define o ambiente (padrão `development`)
- `OTLP_ENDPOINT`: endpoint OTLP para exportação de métricas (opcional)

## Uso da API

Após iniciar o servidor, a API estará disponível em `http://localhost:8080`.
A documentação Swagger pode ser acessada em `http://localhost:8080/swagger/index.html`.

### Iniciar transferência

```bash
curl -X POST http://localhost:8080/api/v1/transfer \
  -H 'Content-Type: application/json' \
  -d '{
    "source": {
      "queueManagerName": "QM1",
      "connectionName": "localhost(1414)",
      "channel": "SYSTEM.DEF.SVRCONN"
    },
    "sourceQueue": "SOURCE.QUEUE",
    "destination": {
      "queueManagerName": "QM1",
      "connectionName": "localhost(1414)",
      "channel": "SYSTEM.DEF.SVRCONN"
    },
    "destinationQueue": "DEST.QUEUE"
  }'
```

### Consultar status

```bash
curl http://localhost:8080/api/v1/transfer/<requestId>
```

### Cancelar transferência

```bash
curl -X POST http://localhost:8080/api/v1/transfer/<requestId>/cancel
```
