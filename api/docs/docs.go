package docs
import "github.com/swaggo/swag"

const docTemplate = `{
    "schemes": {{ marshal .Schemes }},
    "swagger": "2.0",
    "info": {
        "description": "{{escape .Description}}",
        "title": "{{.Title}}",
        "termsOfService": "http://swagger.io/terms/",
        "contact": {
            "name": "API Support",
            "url": "http://www.example.com/support",
            "email": "support@example.com"
        },
        "license": {
            "name": "Apache 2.0",
            "url": "http://www.apache.org/licenses/LICENSE-2.0.html"
        },
        "version": "{{.Version}}"
    },
    "host": "{{.Host}}",
    "basePath": "{{.BasePath}}",
    "paths": {
        "/api/v1/transfer": {
            "post": {
                "description": "Inicia uma transferência de mensagens de uma fila MQ para outra",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "transfer"
                ],
                "summary": "Iniciar transferência de mensagens MQ",
                "parameters": [
                    {
                        "description": "Detalhes da transferência",
                        "name": "request",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/models.TransferRequest"
                        }
                    }
                ],
                "responses": {
                    "202": {
                        "description": "Transferência iniciada",
                        "schema": {
                            "$ref": "#/definitions/models.TransferResponse"
                        }
                    },
                    "400": {
                        "description": "Erro na requisição",
                        "schema": {
                            "$ref": "#/definitions/models.TransferResponse"
                        }
                    },
                    "500": {
                        "description": "Erro interno",
                        "schema": {
                            "$ref": "#/definitions/models.TransferResponse"
                        }
                    }
                }
            }
        },
        "/api/v1/transfer/{requestId}": {
            "get": {
                "description": "Retorna o status atual de uma transferência de mensagens",
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "transfer"
                ],
                "summary": "Obter status da transferência",
                "parameters": [
                    {
                        "type": "string",
                        "description": "ID da requisição de transferência",
                        "name": "requestId",
                        "in": "path",
                        "required": true
                    }
                ],
                "responses": {
                    "200": {
                        "description": "Status da transferência",
                        "schema": {
                            "$ref": "#/definitions/models.TransferStatus"
                        }
                    },
                    "404": {
                        "description": "Transferência não encontrada",
                        "schema": {
                            "$ref": "#/definitions/models.TransferResponse"
                        }
                    }
                }
            }
        },
        "/api/v1/transfer/{requestId}/cancel": {
            "post": {
                "description": "Cancela uma transferência de mensagens que está em andamento",
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "transfer"
                ],
                "summary": "Cancelar uma transferência em andamento",
                "parameters": [
                    {
                        "type": "string",
                        "description": "ID da requisição de transferência",
                        "name": "requestId",
                        "in": "path",
                        "required": true
                    }
                ],
                "responses": {
                    "200": {
                        "description": "Transferência cancelada",
                        "schema": {
                            "$ref": "#/definitions/models.TransferResponse"
                        }
                    },
                    "400": {
                        "description": "Transferência já concluída",
                        "schema": {
                            "$ref": "#/definitions/models.TransferResponse"
                        }
                    },
                    "404": {
                        "description": "Transferência não encontrada",
                        "schema": {
                            "$ref": "#/definitions/models.TransferResponse"
                        }
                    }
                }
            }
        },
        "/api/v1/transfers": {
            "get": {
                "description": "Retorna uma lista com todas as transferências e seus status",
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "transfer"
                ],
                "summary": "Listar todas as transferências",
                "responses": {
                    "200": {
                        "description": "Lista de transferências",
                        "schema": {
                            "type": "array",
                            "items": {
                                "$ref": "#/definitions/models.TransferStatus"
                            }
                        }
                    }
                }
            }
        }
    },
    "definitions": {
        "models.ConnectionDetails": {
            "type": "object",
            "required": [
                "channel",
                "connectionName",
                "queueManagerName"
            ],
            "properties": {
                "channel": {
                    "type": "string",
                    "example": "SYSTEM.DEF.SVRCONN"
                },
                "connectionName": {
                    "type": "string",
                    "example": "localhost(1414)"
                },
                "password": {
                    "type": "string",
                    "example": "mqpassword"
                },
                "queueManagerName": {
                    "type": "string",
                    "example": "QM1"
                },
                "username": {
                    "type": "string",
                    "example": "mquser"
                }
            }
        },
        "models.TransferRequest": {
            "type": "object",
            "required": [
                "destination",
                "destinationQueue",
                "source",
                "sourceQueue"
            ],
            "properties": {
                "bufferSize": {
                    "type": "integer",
                    "example": 1048576
                },
                "commitInterval": {
                    "type": "integer",
                    "example": 10
                },
                "destination": {
                    "$ref": "#/definitions/models.ConnectionDetails"
                },
                "destinationQueue": {
                    "type": "string",
                    "example": "DEST.QUEUE"
                },
                "nonSharedConnection": {
                    "type": "boolean",
                    "example": false
                },
                "source": {
                    "$ref": "#/definitions/models.ConnectionDetails"
                },
                "sourceQueue": {
                    "type": "string",
                    "example": "SOURCE.QUEUE"
                }
            }
        },
        "models.TransferResponse": {
            "type": "object",
            "properties": {
                "error": {
                    "type": "string",
                    "example": "Failed to connect to source queue manager"
                },
                "messagesTotal": {
                    "type": "integer",
                    "example": 1000
                },
                "messagesTransferred": {
                    "type": "integer",
                    "example": 500
                },
                "requestId": {
                    "type": "string",
                    "example": "550e8400-e29b-41d4-a716-446655440000"
                },
                "status": {
                    "type": "string",
                    "example": "in_progress"
                }
            }
        },
        "models.TransferStatus": {
            "type": "object",
            "properties": {
                "bytesTransferred": {
                    "type": "integer",
                    "example": 1048576
                },
                "endTime": {
                    "type": "string",
                    "example": "2025-06-04T00:16:45Z"
                },
                "error": {
                    "type": "string",
                    "example": "Failed to connect to source queue manager"
                },
                "messagesTotal": {
                    "type": "integer",
                    "example": 1000
                },
                "messagesTransferred": {
                    "type": "integer",
                    "example": 500
                },
                "requestId": {
                    "type": "string",
                    "example": "550e8400-e29b-41d4-a716-446655440000"
                },
                "startTime": {
                    "type": "string",
                    "example": "2025-06-04T00:15:30Z"
                },
                "status": {
                    "type": "string",
                    "example": "in_progress"
                }
            }
        }
    }
}`

// SwaggerInfo holds exported Swagger Info so clients can modify it
var SwaggerInfo = &swag.Spec{
	Version:          "1.0",
	Host:             "localhost:8080",
	BasePath:         "/",
	Schemes:          []string{"http"},
	Title:            "MQ Transfer API",
	Description:      "API para transferência de mensagens entre filas IBM MQ",
	InfoInstanceName: "swagger",
	SwaggerTemplate:  docTemplate,
	LeftDelim:        "{{",
	RightDelim:       "}}",
}

func init() {
	swag.Register(SwaggerInfo.InstanceName(), SwaggerInfo)
}