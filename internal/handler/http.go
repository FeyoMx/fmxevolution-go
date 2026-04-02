package handler

import (
	"errors"
	"net/http"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
	"github.com/gin-gonic/gin"
)

type ErrorResponse struct {
	Error string `json:"error"`
}

func WriteJSON(c *gin.Context, status int, payload any) {
	c.JSON(status, payload)
}

func WriteError(c *gin.Context, err error) {
	status := http.StatusInternalServerError

	switch {
	case errors.Is(err, domain.ErrUnauthorized):
		status = http.StatusUnauthorized
	case errors.Is(err, domain.ErrForbidden):
		status = http.StatusForbidden
	case errors.Is(err, domain.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, domain.ErrConflict):
		status = http.StatusConflict
	case errors.Is(err, domain.ErrValidation):
		status = http.StatusBadRequest
	}

	c.JSON(status, ErrorResponse{Error: err.Error()})
}
