package apierror

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// ErrorResponse is the standardized JSON error envelope returned by all endpoints.
type ErrorResponse struct {
	Status    int         `json:"status"`
	Error     string      `json:"error"`
	Message   string      `json:"message,omitempty"`
	RequestID string      `json:"request_id,omitempty"`
	Data      interface{} `json:"data,omitempty"`
}

// New creates an ErrorResponse and sends it as JSON.
func New(c echo.Context, status int, errStr string, message string, requestID string) error {
	resp := ErrorResponse{
		Status:    status,
		Error:     errStr,
		RequestID: requestID,
	}
	if message != "" {
		resp.Message = message
	}
	return c.JSON(status, resp)
}

// WithData creates an ErrorResponse with additional data and sends it as JSON.
func WithData(c echo.Context, status int, errStr string, message string, requestID string, data interface{}) error {
	resp := ErrorResponse{
		Status:    status,
		Error:     errStr,
		RequestID: requestID,
	}
	if message != "" {
		resp.Message = message
	}
	if data != nil {
		resp.Data = data
	}
	return c.JSON(status, resp)
}

// CustomHTTPErrorHandler handles Echo-generated errors (404, 405, etc.)
// using the standardized error envelope. The requestIDFunc extracts the
// request ID from the echo.Context.
func CustomHTTPErrorHandler(requestIDFunc func(echo.Context) string) func(error, echo.Context) {
	return func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}

		he, ok := err.(*echo.HTTPError)
		if !ok {
			he = echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}

		status := he.Code
		errStr := http.StatusText(status)
		message := ""

		if m, ok := he.Message.(string); ok {
			message = m
		}

		reqID := ""
		if requestIDFunc != nil {
			reqID = requestIDFunc(c)
		}

		resp := ErrorResponse{
			Status:    status,
			Error:     errStr,
			RequestID: reqID,
		}
		if message != "" {
			resp.Message = message
		}

		if c.Request().Method == http.MethodHead {
			c.NoContent(status)
		} else {
			c.JSON(status, resp)
		}
	}
}
