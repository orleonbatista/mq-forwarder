# MQ Forwarder

Aplicação escrita em Go para transferir mensagens entre filas IBM MQ. Ela expõe uma API REST usando o framework Gin e registra métricas opcionais via OpenTelemetry.

A lógica de transferência está no pacote `internal/transfer`, que possui uma implementação simplificada para facilitar testes. A integração real com o MQ só é ativada quando o projeto é compilado com as bibliotecas C do IBM MQ (tag de build `ibmmq`).

Para instruções de instalação e exemplos de uso, consulte o arquivo [GETTING_STARTED.md](GETTING_STARTED.md).
