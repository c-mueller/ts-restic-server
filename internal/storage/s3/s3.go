package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/chrismcg/ts-restic-server/internal/storage"
)

type Backend struct {
	client *s3.Client
	bucket string
	prefix string
}

func New(ctx context.Context, bucket, prefix, region, endpoint string) (*Backend, error) {
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(region),
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}

	clientOpts := []func(*s3.Options){}
	if endpoint != "" {
		clientOpts = append(clientOpts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true
		})
	}

	client := s3.NewFromConfig(cfg, clientOpts...)

	return &Backend{
		client: client,
		bucket: bucket,
		prefix: prefix,
	}, nil
}

func (b *Backend) CreateRepo(ctx context.Context) error {
	// Create a marker object to indicate repo exists
	_, err := b.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(b.key("repo.marker")),
		Body:   bytes.NewReader([]byte{}),
	})
	return err
}

func (b *Backend) DeleteRepo(ctx context.Context) error {
	// List and delete all objects with our prefix
	paginator := s3.NewListObjectsV2Paginator(b.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(b.bucket),
		Prefix: aws.String(b.prefixPath()),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("list objects: %w", err)
		}

		if len(page.Contents) == 0 {
			continue
		}

		objects := make([]types.ObjectIdentifier, len(page.Contents))
		for i, obj := range page.Contents {
			objects[i] = types.ObjectIdentifier{Key: obj.Key}
		}

		_, err = b.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(b.bucket),
			Delete: &types.Delete{Objects: objects},
		})
		if err != nil {
			return fmt.Errorf("delete objects: %w", err)
		}
	}

	return nil
}

func (b *Backend) StatConfig(ctx context.Context) (int64, error) {
	out, err := b.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(b.key("config")),
	})
	if err != nil {
		if isNotFound(err) {
			return 0, storage.ErrNotFound
		}
		return 0, err
	}
	if out.ContentLength == nil {
		return 0, nil
	}
	return *out.ContentLength, nil
}

func (b *Backend) GetConfig(ctx context.Context) (io.ReadCloser, error) {
	out, err := b.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(b.key("config")),
	})
	if err != nil {
		if isNotFound(err) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	return out.Body, nil
}

func (b *Backend) SaveConfig(ctx context.Context, data io.Reader) error {
	buf, err := io.ReadAll(data)
	if err != nil {
		return err
	}

	_, err = b.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(b.key("config")),
		Body:   bytes.NewReader(buf),
	})
	return err
}

func (b *Backend) StatBlob(ctx context.Context, t storage.BlobType, name string) (int64, error) {
	out, err := b.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(b.blobKey(t, name)),
	})
	if err != nil {
		if isNotFound(err) {
			return 0, storage.ErrNotFound
		}
		return 0, err
	}
	if out.ContentLength == nil {
		return 0, nil
	}
	return *out.ContentLength, nil
}

func (b *Backend) GetBlob(ctx context.Context, t storage.BlobType, name string, offset, length int64) (io.ReadCloser, error) {
	input := &s3.GetObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(b.blobKey(t, name)),
	}

	if offset > 0 || length > 0 {
		var rangeStr string
		if length > 0 {
			rangeStr = fmt.Sprintf("bytes=%d-%d", offset, offset+length-1)
		} else {
			rangeStr = fmt.Sprintf("bytes=%d-", offset)
		}
		input.Range = aws.String(rangeStr)
	}

	out, err := b.client.GetObject(ctx, input)
	if err != nil {
		if isNotFound(err) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	return out.Body, nil
}

func (b *Backend) SaveBlob(ctx context.Context, t storage.BlobType, name string, data io.Reader) error {
	buf, err := io.ReadAll(data)
	if err != nil {
		return err
	}

	_, err = b.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(b.blobKey(t, name)),
		Body:   bytes.NewReader(buf),
	})
	return err
}

func (b *Backend) DeleteBlob(ctx context.Context, t storage.BlobType, name string) error {
	// Check existence first (S3 DeleteObject doesn't error on missing keys)
	_, err := b.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(b.blobKey(t, name)),
	})
	if err != nil {
		if isNotFound(err) {
			return storage.ErrNotFound
		}
		return err
	}

	_, err = b.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(b.blobKey(t, name)),
	})
	return err
}

func (b *Backend) ListBlobs(ctx context.Context, t storage.BlobType) ([]storage.Blob, error) {
	prefix := b.key(string(t)) + "/"
	var blobs []storage.Blob

	paginator := s3.NewListObjectsV2Paginator(b.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(b.bucket),
		Prefix: aws.String(prefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list objects: %w", err)
		}

		for _, obj := range page.Contents {
			name := path.Base(*obj.Key)
			size := int64(0)
			if obj.Size != nil {
				size = *obj.Size
			}
			blobs = append(blobs, storage.Blob{Name: name, Size: size})
		}
	}

	if blobs == nil {
		blobs = []storage.Blob{}
	}
	return blobs, nil
}

func (b *Backend) prefixPath() string {
	if b.prefix != "" {
		return b.prefix + "/"
	}
	return ""
}

func (b *Backend) key(parts ...string) string {
	k := path.Join(parts...)
	if b.prefix != "" {
		k = b.prefix + "/" + k
	}
	return k
}

func (b *Backend) blobKey(t storage.BlobType, name string) string {
	return b.key(string(t), name)
}

func isNotFound(err error) bool {
	var nsk *types.NoSuchKey
	var nsb *types.NotFound
	if ok := errors.As(err, &nsk); ok {
		return true
	}
	if ok := errors.As(err, &nsb); ok {
		return true
	}
	// Also check the error message for "NotFound" or "NoSuchKey" since the SDK
	// sometimes wraps these differently
	return strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "NoSuchKey")
}
