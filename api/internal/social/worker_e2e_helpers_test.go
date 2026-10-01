package social

// This file exposes a small constructor surface for the E1/E2
// integration tests that live in `package social_test`. The Worker's
// `NewWorker` takes unexported interface parameters
// (workerRepoIface, instagramIface, storageIface), which the Go
// compiler refuses to accept from another package even when a
// candidate type structurally satisfies them — Go's type checker
// requires the candidate interface to be visible at the call site
// for implicit satisfaction.
//
// We solve that by adding three exported stub interfaces here, plus
// an exported constructor (NewWorkerForTest) and tick driver
// (TickWorkerForTest). The constructor accepts those exported
// interfaces; the implementation assigns them to the Worker's
// unexported fields, where the structural match is checked inside
// this package and accepted.
//
// The companion test file is `worker_e2e_integration_test.go` in
// `package social_test`. That file shares the testPool provisioned
// by `repository_test.go`'s TestMain, so no new TestMain is needed
// here — that would collide with the existing one in the same
// compile target.

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// WorkerDepsForTest is the exported mirror of the unexported
// workerRepoIface. The test code implements these methods on a
// concrete stub type and passes it to NewWorkerForTest.
type WorkerRepoStub interface {
	ListDuePosts(ctx context.Context) ([]ScheduledPost, error)
	ListExpiringAccounts(ctx context.Context) ([]SocialAccount, error)
	UpdatePostStatus(ctx context.Context, id uuid.UUID, status PostStatus, errReason string) error
	SetNextRetry(ctx context.Context, id uuid.UUID, retryCount int, nextRetryAt time.Time) error
	UpdateContainerID(ctx context.Context, id uuid.UUID, containerID string) error
	UpdateAccountStatus(ctx context.Context, id uuid.UUID, status string) error
	UpdateAccountToken(ctx context.Context, id uuid.UUID, encryptedToken string, expiry time.Time) error
	GetAccount(ctx context.Context) (*SocialAccount, error)
}

// WorkerStorageStub is the exported mirror of the unexported
// storageIface. The Worker's presign call goes through this type.
type WorkerStorageStub interface {
	PresignedGetObject(ctx context.Context, bucketName, objectName string, expiry time.Duration, reqParams map[string]string) (string, error)
	Bucket() string
}

// WorkerInstagramStub is the exported mirror of the unexported
// instagramIface.
type WorkerInstagramStub interface {
	CreateContainer(ctx context.Context, igUserID, accessToken, imageURL, caption string, postType PostType) (string, error)
	PublishContainer(ctx context.Context, igUserID, accessToken, containerID string) (string, error)
	CheckContainerStatus(ctx context.Context, containerID, accessToken string) (ContainerStatus, error)
	RefreshToken(ctx context.Context, currentToken string) (string, time.Time, error)
}

// NewWorkerForTest is the exported constructor. It mirrors NewWorker
// but uses the exported stub interfaces so callers in other packages
// can satisfy them.
func NewWorkerForTest(
	repo WorkerRepoStub,
	ig WorkerInstagramStub,
	storage WorkerStorageStub,
	cryptoKey []byte,
	log zerolog.Logger,
) *Worker {
	// The structural assignment from the exported interface to the
	// unexported worker field type happens here, inside the same
	// package as the Worker struct. The Go compiler accepts this
	// because both interface types have identical method sets.
	return &Worker{
		repo:      repo,
		instagram: ig,
		storage:   storage,
		cryptoKey: cryptoKey,
		log:       log,
	}
}

// TickWorkerForTest invokes the Worker's tick method once. Mirrors
// a single 60s ticker iteration. Exported so cross-package e2e tests
// can drive the worker without poking at the unexported method
// directly.
func TickWorkerForTest(w *Worker, ctx context.Context) error {
	return w.tick(ctx)
}
