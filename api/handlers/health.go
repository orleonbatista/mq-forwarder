package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"mq-transfer-go/api/models"
)

// HealthCheck retorna o status geral da aplicação
// @Summary Health Check
// @Description Retorna informacoes de saude da aplicacao
// @Tags health
// @Produce json
// @Success 200 {object} models.HealthResponse
// @Router /api/v1/health [get]
func HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, models.HealthResponse{
		Status:  "ok",
		Version: "1.0.0",
	})
}
