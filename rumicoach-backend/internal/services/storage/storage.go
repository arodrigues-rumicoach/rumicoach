// Package storage holds the object store for user-uploaded binaries — today the images
// attached to feedback reports.
//
// The bucket is configured per deployment rather than chosen per request. Each data plane
// is its own Cloud Run service with its own environment, so pointing FEEDBACK_BUCKET at a
// bucket in that plane's region keeps residency correct by construction: a European user's
// screenshot cannot end up in a US bucket, because the code that stores it never runs
// there. No region branching, nothing to get wrong later.
package storage

import (
	"context"
	"fmt"
	"io"
	"time"

	gcs "cloud.google.com/go/storage"
	"go.uber.org/zap"
)

// Store is the object store. Deployments without a bucket configured (local development,
// tests) get a no-op implementation that logs instead of writing, mirroring how the
// messaging package degrades to a mock channel.
type Store interface {
	// Upload writes data at path and returns the path it was stored under.
	Upload(ctx context.Context, path, contentType string, data []byte) (string, error)
	// Delete removes one object. Deleting something that is not there is not an error:
	// callers delete after a database transaction has already committed, so a retry must
	// not fail on the second pass.
	Delete(ctx context.Context, path string) error
	// DeletePrefix removes every object under a prefix, for erasing a user at once.
	DeletePrefix(ctx context.Context, prefix string) error
	// SignedURL returns a time-limited read URL, so images can be linked from an email
	// without the bucket being public.
	SignedURL(ctx context.Context, path string, ttl time.Duration) (string, error)
	// Enabled reports whether objects are really being stored.
	Enabled() bool
}

// Global is the process-wide store, set by main at startup.
var Global Store = NoopStore{}

type gcsStore struct {
	client *gcs.Client
	bucket string
	logger *zap.Logger
}

// New returns a GCS-backed store, or a no-op one when bucket is empty.
func New(ctx context.Context, bucket string, logger *zap.Logger) (Store, error) {
	if bucket == "" {
		// Loud, but once. A deployment running without a bucket has a silently degraded
		// feature, and boot is the right place to say so — not every upload.
		logger.Warn("FEEDBACK_BUCKET is not set: feedback screenshots will be discarded")
		return NoopStore{logger: logger}, nil
	}
	client, err := gcs.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("storage: %w", err)
	}
	logger.Info("object storage configured", zap.String("bucket", bucket))
	return &gcsStore{client: client, bucket: bucket, logger: logger}, nil
}

func (s *gcsStore) Enabled() bool { return true }

func (s *gcsStore) Upload(ctx context.Context, path, contentType string, data []byte) (string, error) {
	w := s.client.Bucket(s.bucket).Object(path).NewWriter(ctx)
	w.ContentType = contentType
	// Uploads are user-supplied binaries. Serving them inline would let a crafted file run
	// in the browser's origin if the bucket were ever exposed; as an attachment it cannot.
	w.ContentDisposition = "attachment"
	if _, err := w.Write(data); err != nil {
		_ = w.Close()
		return "", fmt.Errorf("storage: write %s: %w", path, err)
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("storage: close %s: %w", path, err)
	}
	return path, nil
}

func (s *gcsStore) Delete(ctx context.Context, path string) error {
	err := s.client.Bucket(s.bucket).Object(path).Delete(ctx)
	if err == gcs.ErrObjectNotExist {
		return nil
	}
	return err
}

func (s *gcsStore) DeletePrefix(ctx context.Context, prefix string) error {
	it := s.client.Bucket(s.bucket).Objects(ctx, &gcs.Query{Prefix: prefix})
	for {
		attrs, err := it.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := s.Delete(ctx, attrs.Name); err != nil {
			return err
		}
	}
}

func (s *gcsStore) SignedURL(ctx context.Context, path string, ttl time.Duration) (string, error) {
	return s.client.Bucket(s.bucket).SignedURL(path, &gcs.SignedURLOptions{
		Method:  "GET",
		Expires: time.Now().Add(ttl),
	})
}

// NoopStore discards uploads. It exists so local development and tests run without a
// bucket, and so a missing configuration degrades to "the image was not kept" instead of
// failing the whole request — a feedback report without its screenshot is still worth
// having.
type NoopStore struct{ logger *zap.Logger }

func (n NoopStore) Enabled() bool { return false }

func (n NoopStore) Upload(_ context.Context, path, contentType string, data []byte) (string, error) {
	// Info, not Warn: this is a configured state, not something going wrong, and the
	// development logger attaches a full stack trace to everything at Warn and above.
	// Thirty frames of trace on a request that returned 201 reads as a failure and buries
	// the logs that matter. Startup already says it loudly, once.
	if n.logger != nil {
		n.logger.Info("no bucket configured; feedback image discarded",
			zap.String("path", path), zap.String("contentType", contentType), zap.Int("bytes", len(data)))
	}
	return "", nil
}

func (n NoopStore) Delete(context.Context, string) error       { return nil }
func (n NoopStore) DeletePrefix(context.Context, string) error { return nil }
func (n NoopStore) SignedURL(context.Context, string, time.Duration) (string, error) {
	return "", fmt.Errorf("storage: not configured")
}
