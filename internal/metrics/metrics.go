package metrics

import (
	"github.com/c-mueller/ts-restic-server/internal/buildinfo"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Registry is the custom Prometheus registry used by the server.
var Registry *prometheus.Registry

var (
	BuildInfo *prometheus.GaugeVec

	HTTPRequestDuration *prometheus.HistogramVec
	HTTPRequestsTotal   *prometheus.CounterVec
	HTTPErrorsTotal     *prometheus.CounterVec

	ACLDecisionsTotal     *prometheus.CounterVec
	ACLEvaluationDuration *prometheus.HistogramVec

	HostRequestsTotal      *prometheus.CounterVec
	HostBytesReceivedTotal *prometheus.CounterVec
	HostBytesSentTotal     *prometheus.CounterVec

	StorageOperationDuration *prometheus.HistogramVec
	StorageOperationsTotal   *prometheus.CounterVec

	PanicsTotal prometheus.Counter
)

// Init creates the custom registry and registers all metrics.
func Init(backendName string) {
	Registry = prometheus.NewRegistry()
	Registry.MustRegister(collectors.NewGoCollector())
	Registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	BuildInfo = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "restic_server_build_info",
		Help: "Build information for the restic server.",
	}, []string{"version", "commit", "build_date"})
	BuildInfo.WithLabelValues(buildinfo.Version, buildinfo.Commit, buildinfo.BuildDate).Set(1)

	HTTPRequestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "restic_server_http_request_duration_seconds",
		Help:    "Duration of HTTP requests in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path", "status_code"})

	HTTPRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "restic_server_http_requests_total",
		Help: "Total number of HTTP requests.",
	}, []string{"method", "path", "status_code"})

	HTTPErrorsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "restic_server_http_errors_total",
		Help: "Total number of HTTP error responses (4xx and 5xx).",
	}, []string{"method", "path", "status_code"})

	ACLDecisionsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "restic_server_acl_decisions_total",
		Help: "Total number of ACL decisions.",
	}, []string{"result"})

	ACLEvaluationDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "restic_server_acl_evaluation_duration_seconds",
		Help:    "Duration of ACL evaluations in seconds.",
		Buckets: []float64{0.00001, 0.00005, 0.0001, 0.0005, 0.001, 0.005, 0.01},
	}, []string{"result"})

	HostRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "restic_server_host_requests_total",
		Help: "Total number of requests per identity, repo path, and method.",
	}, []string{"identity", "repo_path", "method"})

	HostBytesReceivedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "restic_server_host_bytes_received_total",
		Help: "Total bytes received per identity and repo path.",
	}, []string{"identity", "repo_path"})

	HostBytesSentTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "restic_server_host_bytes_sent_total",
		Help: "Total bytes sent per identity and repo path.",
	}, []string{"identity", "repo_path"})

	StorageOperationDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "restic_server_storage_operation_duration_seconds",
		Help:    "Duration of storage backend operations in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"operation", "backend"})

	StorageOperationsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "restic_server_storage_operations_total",
		Help: "Total number of storage backend operations.",
	}, []string{"operation", "backend", "result"})

	PanicsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "restic_server_panics_total",
		Help: "Total number of recovered panics.",
	})

	Registry.MustRegister(
		BuildInfo,
		HTTPRequestDuration,
		HTTPRequestsTotal,
		HTTPErrorsTotal,
		ACLDecisionsTotal,
		ACLEvaluationDuration,
		HostRequestsTotal,
		HostBytesReceivedTotal,
		HostBytesSentTotal,
		StorageOperationDuration,
		StorageOperationsTotal,
		PanicsTotal,
	)
}
