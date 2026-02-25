package webdav_test

import (
	"fmt"
	"net"
	"net/http"
	"testing"

	"github.com/c-mueller/ts-restic-server/internal/storage"
	"github.com/c-mueller/ts-restic-server/internal/storage/backendtest"
	webdavbackend "github.com/c-mueller/ts-restic-server/internal/storage/webdav"
	"golang.org/x/net/webdav"
)

func startWebDAVServer(t *testing.T) string {
	t.Helper()
	handler := &webdav.Handler{
		FileSystem: webdav.NewMemFS(),
		LockSystem: webdav.NewMemLS(),
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	srv := &http.Server{Handler: handler}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })

	return fmt.Sprintf("http://%s", ln.Addr().String())
}

func TestSuite(t *testing.T) {
	backendtest.RunSuite(t, func(t *testing.T) storage.Backend {
		endpoint := startWebDAVServer(t)
		return webdavbackend.New(endpoint, "", "", "", false)
	})
}
