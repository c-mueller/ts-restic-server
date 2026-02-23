package api

import (
	"github.com/c-mueller/ts-restic-server/internal/storage"
	"go.uber.org/zap"
)

type Handler struct {
	Backend    storage.Backend
	Logger     *zap.Logger
	AppendOnly bool
}
