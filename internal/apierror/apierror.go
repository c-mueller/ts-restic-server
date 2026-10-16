package apierror

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"
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
func New(c *echo.Context, status int, errStr string, message string, requestID string) error {
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
func WithData(c *echo.Context, status int, errStr string, message string, requestID string, data interface{}) error {
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
// request ID from the *echo.Context.
func CustomHTTPErrorHandler(requestIDFunc func(*echo.Context) string) echo.HTTPErrorHandler {
	return func(c *echo.Context, err error) {
		if r, _ := echo.UnwrapResponse(c.Response()); r != nil && r.Committed {
			return
		}

		status := echo.StatusCode(err)
		var message string
		var he *echo.HTTPError
		switch {
		case errors.As(err, &he):
			message = he.Message
		case status == 0:
			status = http.StatusInternalServerError
			message = err.Error()
		}
		if message == "" {
			message = http.StatusText(status)
		}
		errStr := http.StatusText(status)

		reqID := ""
		if requestIDFunc != nil {
			reqID = requestIDFunc(c)
		}

		resp := ErrorResponse{
			Status:    status,
			Error:     errStr,
			Message:   message,
			RequestID: reqID,
		}

		if c.Request().Method == http.MethodHead {
			c.NoContent(status)
		} else {
			c.JSON(status, resp)
		}
	}
}
