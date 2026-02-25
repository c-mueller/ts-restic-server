package api

import (
	"strings"

	"github.com/labstack/echo/v4"
)

const (
	resticV1Type = "application/vnd.x.restic.rest.v1"
	resticV2Type = "application/vnd.x.restic.rest.v2"
)

func isV2(c echo.Context) bool {
	return strings.EqualFold(c.Request().Header.Get("Accept"), resticV2Type)
}
