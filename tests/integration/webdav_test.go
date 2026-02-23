package integration

import (
	"fmt"
	"net"
	"net/http"
	"testing"

	webdavbackend "github.com/c-mueller/ts-restic-server/internal/storage/webdav"
	"golang.org/x/net/webdav"
)

// startWebDAVServer starts an in-process WebDAV server backed by an in-memory
// filesystem and returns its base URL. No Docker required.
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

func TestWebDAVBackend(t *testing.T) {
	t.Parallel()
	endpoint := startWebDAVServer(t)
	backend := webdavbackend.New(endpoint, "", "", "", false)
	runBackendSuite(t, backend)
}
