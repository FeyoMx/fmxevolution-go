package handler

import (
	"errors"
	"net/http"

	"github.com/EvolutionAPI/evolution-go/internal/domain"
	"github.com/gin-gonic/gin"
)

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Code    string `json:"code"`
}

func WriteJSON(c *gin.Context, status int, payload any) {
	c.JSON(status, payload)
}

func WriteValidationError(c *gin.Context, message string, err error) {
	if err == nil {
		err = domain.ErrValidation
	}
	c.JSON(http.StatusBadRequest, ErrorResponse{
		Error:   err.Error(),
		Message: message,
		Code:    "validation_failed",
	})
}

func WriteError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	code := "internal_error"

	switch {
	case errors.Is(err, domain.ErrUnauthorized):
		status = http.StatusUnauthorized
		code = "unauthorized"
	case errors.Is(err, domain.ErrForbidden):
		status = http.StatusForbidden
		code = "forbidden"
	case errors.Is(err, domain.ErrNotFound):
		status = http.StatusNotFound
		code = "not_found"
	case errors.Is(err, domain.ErrConflict):
		status = http.StatusConflict
		code = "conflict"
	case errors.Is(err, domain.ErrValidation):
		status = http.StatusBadRequest
		code = "validation_failed"
	case errors.Is(err, domain.ErrTimeout):
		status = http.StatusGatewayTimeout
		code = "timeout"
	}

	c.JSON(status, ErrorResponse{
		Error:   err.Error(),
		Message: err.Error(),
		Code:    code,
	})
}
