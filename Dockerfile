# Etapa 1: build
FROM golang:1.23-bullseye AS builder

RUN apt-get update && apt-get install -y curl tar && rm -rf /var/lib/apt/lists/*

ENV MQ_TAR="9.4.3.0-IBM-MQC-Redist-LinuxX64.tar.gz"
ENV MQ_URL="https://public.dhe.ibm.com/ibmdl/export/pub/software/websphere/messaging/mqdev/redist/${MQ_TAR}"
ENV genmqpkg_incnls=1 genmqpkg_incsdk=1 genmqpkg_inctls=1

RUN mkdir -p /opt/mqm && cd /opt/mqm \
    && curl -LO "$MQ_URL" \
    && tar -zxf "$MQ_TAR" \
    && rm "$MQ_TAR" \
    && bin/genmqpkg.sh -b /opt/mqm

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -tags ibmmq -o mq-forwarder main.go

# Etapa 2: imagem final mais leve
FROM debian:bullseye-slim

COPY --from=builder /opt/mqm /opt/mqm
COPY --from=builder /app/mq-forwarder /usr/local/bin/mq-forwarder

RUN mkdir -p /IBM/MQ/data/errors /.mqm \
    && chmod -R 777 /IBM /opt/mqm /.mqm \
    && useradd -u 1000 -m appuser

USER appuser
WORKDIR /home/appuser


EXPOSE 8080
CMD ["mq-forwarder"]
