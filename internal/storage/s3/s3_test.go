package s3_test

import (
	"context"
	"fmt"
	"os/exec"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	s3client "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/c-mueller/ts-restic-server/internal/storage"
	"github.com/c-mueller/ts-restic-server/internal/storage/backendtest"
	s3backend "github.com/c-mueller/ts-restic-server/internal/storage/s3"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping S3 test in short mode")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker not available, skipping S3 test")
	}

	ctx := context.Background()

	const (
		accessKey = "minioadmin" // pragma: allowlist secret
		secretKey = "minioadmin" // pragma: allowlist secret
		region    = "us-east-1"
	)

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "minio/minio:latest",
			ExposedPorts: []string{"9000/tcp"},
			Env: map[string]string{
				"MINIO_ROOT_USER":     accessKey,
				"MINIO_ROOT_PASSWORD": secretKey,
			},
			Cmd:        []string{"server", "/data"},
			WaitingFor: wait.ForHTTP("/minio/health/live").WithPort("9000"),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start minio container: %v", err)
	}
	t.Cleanup(func() { container.Terminate(ctx) })

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("get container host: %v", err)
	}
	mappedPort, err := container.MappedPort(ctx, "9000")
	if err != nil {
		t.Fatalf("get mapped port: %v", err)
	}
	endpoint := fmt.Sprintf("http://%s:%s", host, mappedPort.Port())

	bucketCounter := 0

	backendtest.RunSuite(t, func(t *testing.T) storage.Backend {
		bucketCounter++
		bucket := fmt.Sprintf("test-bucket-%d", bucketCounter)

		cfg, err := awsconfig.LoadDefaultConfig(ctx,
			awsconfig.WithRegion(region),
			awsconfig.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
			),
		)
		if err != nil {
			t.Fatalf("aws config: %v", err)
		}

		s3c := s3client.NewFromConfig(cfg, func(o *s3client.Options) {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true
		})
		if _, err := s3c.CreateBucket(ctx, &s3client.CreateBucketInput{
			Bucket: aws.String(bucket),
		}); err != nil {
			t.Fatalf("create bucket: %v", err)
		}

		backend, err := s3backend.New(ctx, bucket, "", region, endpoint, accessKey, secretKey)
		if err != nil {
			t.Fatalf("create s3 backend: %v", err)
		}
		return backend
	})
}
