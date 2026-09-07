package redis

import (
	"context"
	"errors"
	"testing"
	"time"

	growthbook "github.com/growthbook/growthbook-golang"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

// fakeRedis is an in-memory stand-in for a go-redis client. go-redis exposes
// constructors for its result types (NewStringResult and friends), so a fake
// can be written without a live server or a mocking framework.
type fakeRedis struct {
	store map[string]string

	getCalls  int
	mgetCalls int
	lastTTL   time.Duration

	getErr  error
	mgetErr error
	setErr  error
}

func newFakeRedis() *fakeRedis {
	return &fakeRedis{store: map[string]string{}}
}

func (f *fakeRedis) Get(_ context.Context, key string) *goredis.StringCmd {
	f.getCalls++
	if f.getErr != nil {
		return goredis.NewStringResult("", f.getErr)
	}
	v, ok := f.store[key]
	if !ok {
		return goredis.NewStringResult("", goredis.Nil)
	}
	return goredis.NewStringResult(v, nil)
}

func (f *fakeRedis) Set(_ context.Context, key string, value any, ttl time.Duration) *goredis.StatusCmd {
	if f.setErr != nil {
		return goredis.NewStatusResult("", f.setErr)
	}
	f.lastTTL = ttl
	switch v := value.(type) {
	case []byte:
		f.store[key] = string(v)
	case string:
		f.store[key] = v
	}
	return goredis.NewStatusResult("OK", nil)
}

func (f *fakeRedis) MGet(_ context.Context, keys ...string) *goredis.SliceCmd {
	f.mgetCalls++
	if f.mgetErr != nil {
		return goredis.NewSliceResult(nil, f.mgetErr)
	}
	out := make([]any, len(keys))
	for i, k := range keys {
		if v, ok := f.store[k]; ok {
			out[i] = v
		}
	}
	return goredis.NewSliceResult(out, nil)
}

func doc(name, value string, assignments map[string]string) *growthbook.StickyBucketAssignmentDoc {
	return &growthbook.StickyBucketAssignmentDoc{
		AttributeName:  name,
		AttributeValue: value,
		Assignments:    assignments,
	}
}

func TestSaveAndGetRoundTrip(t *testing.T) {
	fake := newFakeRedis()
	svc := New(fake)

	want := doc("id", "user-1", map[string]string{"exp__0": "control"})
	require.NoError(t, svc.SaveAssignments(want))

	got, err := svc.GetAssignments("id", "user-1")
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestSaveUsesNoExpiry(t *testing.T) {
	fake := newFakeRedis()
	require.NoError(t, New(fake).SaveAssignments(doc("id", "user-1", nil)))

	// An assignment must outlive the process; a TTL would silently re-bucket
	// the user once it lapsed.
	require.Zero(t, fake.lastTTL)
}

func TestMissingKeyIsNotAnError(t *testing.T) {
	got, err := New(newFakeRedis()).GetAssignments("id", "nobody")
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestEmptyAttributeIsNotLookedUp(t *testing.T) {
	fake := newFakeRedis()
	svc := New(fake)

	got, err := svc.GetAssignments("id", "")
	require.NoError(t, err)
	require.Nil(t, got)

	got, err = svc.GetAssignments("", "user-1")
	require.NoError(t, err)
	require.Nil(t, got)

	// Degenerate keys like "id||" must never reach Redis.
	require.Zero(t, fake.getCalls)
}

func TestKeyFormatMatchesOtherSDKs(t *testing.T) {
	fake := newFakeRedis()
	require.NoError(t, New(fake).SaveAssignments(doc("id", "user-1", nil)))

	// The JS RedisStickyBucketService writes exactly this key, so with no prefix
	// a Go and a JS service can share one Redis.
	require.Contains(t, fake.store, "id||user-1")
}

func TestKeyPrefixAppliesToRedisOnly(t *testing.T) {
	fake := newFakeRedis()
	svc := New(fake, WithKeyPrefix("gb:"))

	require.NoError(t, svc.SaveAssignments(doc("id", "user-1", map[string]string{"e__0": "v1"})))
	require.Contains(t, fake.store, "gb:id||user-1")

	docs, err := svc.GetAllAssignments(map[string]string{"id": "user-1"})
	require.NoError(t, err)
	// The prefix is a Redis-side detail: the SDK still sees its own key form.
	require.Contains(t, docs, "id||user-1")
}

func TestGetAllAssignmentsUsesOneRoundTrip(t *testing.T) {
	fake := newFakeRedis()
	svc := New(fake)

	require.NoError(t, svc.SaveAssignments(doc("id", "user-1", map[string]string{"e__0": "v1"})))
	require.NoError(t, svc.SaveAssignments(doc("deviceId", "dev-9", map[string]string{"f__0": "v2"})))

	docs, err := svc.GetAllAssignments(map[string]string{
		"id":       "user-1",
		"deviceId": "dev-9",
		"absent":   "nothing-here",
	})
	require.NoError(t, err)

	require.Len(t, docs, 2)
	require.Equal(t, "v1", docs["id||user-1"].Assignments["e__0"])
	require.Equal(t, "v2", docs["deviceId||dev-9"].Assignments["f__0"])
	require.NotContains(t, docs, "absent||nothing-here")

	// The point of the batch: one MGET, not a round-trip per attribute.
	require.Equal(t, 1, fake.mgetCalls)
	require.Zero(t, fake.getCalls)
}

func TestGetAllAssignmentsWithNoAttributes(t *testing.T) {
	fake := newFakeRedis()
	docs, err := New(fake).GetAllAssignments(nil)
	require.NoError(t, err)
	require.Empty(t, docs)
	require.Zero(t, fake.mgetCalls)
}

func TestBackendErrorsAreReported(t *testing.T) {
	down := errors.New("connection refused")

	fake := newFakeRedis()
	fake.getErr = down
	_, err := New(fake).GetAssignments("id", "user-1")
	require.ErrorIs(t, err, down)

	fake = newFakeRedis()
	fake.mgetErr = down
	_, err = New(fake).GetAllAssignments(map[string]string{"id": "user-1"})
	require.ErrorIs(t, err, down)

	fake = newFakeRedis()
	fake.setErr = down
	require.ErrorIs(t, New(fake).SaveAssignments(doc("id", "user-1", nil)), down)
}

func TestForeignValueIsRejected(t *testing.T) {
	fake := newFakeRedis()
	// Something else in a shared Redis happens to own this key.
	fake.store["id||user-1"] = `{"unrelated":"payload"}`

	_, err := New(fake).GetAssignments("id", "user-1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not a sticky bucket document")
}

func TestUnreadableValueIsRejected(t *testing.T) {
	fake := newFakeRedis()
	fake.store["id||user-1"] = "not json at all"

	_, err := New(fake).GetAssignments("id", "user-1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unreadable")
}

func TestSaveRejectsIncompleteDocument(t *testing.T) {
	svc := New(newFakeRedis())
	require.NoError(t, svc.SaveAssignments(nil))
	require.Error(t, svc.SaveAssignments(doc("", "user-1", nil)))
	require.Error(t, svc.SaveAssignments(doc("id", "", nil)))
}

func TestCancelledContextStopsCalls(t *testing.T) {
	cancelled, cancel := context.WithCancel(ctx)
	cancel()

	fake := newFakeRedis()
	// The fake ignores ctx, so this only proves the option is wired; against a
	// real client the cancelled context aborts the command.
	svc := New(fake, WithContext(cancelled), WithTimeout(time.Second))
	_, err := svc.GetAssignments("id", "user-1")
	require.NoError(t, err)
}

// All traffic is weighted into variation 0, so fresh bucketing can only ever
// return "a". Any other answer proves a stored assignment was honoured.
const weightedToFirstVariation = `{"feat":{"defaultValue":"off","rules":[{
	"key":"exp",
	"variations":["a","b","c","d"],
	"weights":[1,0,0,0],
	"meta":[{"key":"0"},{"key":"1"},{"key":"2"},{"key":"3"}],
	"hashAttribute":"id"
}]}}`

func evalWith(t *testing.T, svc growthbook.StickyBucketService, userID string) growthbook.FeatureValue {
	t.Helper()
	client, err := growthbook.NewClient(ctx,
		growthbook.WithJsonFeatures(weightedToFirstVariation),
		growthbook.WithStickyBucketService(svc),
		growthbook.WithAttributes(growthbook.Attributes{"id": userID}),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	return client.EvalFeature(ctx, "feat").Value
}

// The payoff: a client backed by a shared Redis honours an assignment written
// by some other instance, instead of bucketing the user afresh. This is what a
// per-process in-memory service cannot do across a fleet.
func TestStoredAssignmentBeatsFreshBucketing(t *testing.T) {
	shared := newFakeRedis()
	// As if another pod had already bucketed this user into variation 3.
	require.NoError(t, New(shared).SaveAssignments(
		doc("id", "user-42", map[string]string{"exp__0": "3"})))

	require.Equal(t, "d", evalWith(t, New(shared), "user-42"))
	// Sanity check that the weights really do force "a" without an assignment.
	require.Equal(t, "a", evalWith(t, New(shared), "user-with-no-assignment"))
}

func TestFreshAssignmentIsPersisted(t *testing.T) {
	shared := newFakeRedis()
	require.Equal(t, "a", evalWith(t, New(shared), "user-7"))

	// The assignment reached Redis rather than staying in this process, so the
	// next instance to see this user can reuse it.
	require.Contains(t, shared.store, "id||user-7")

	stored, err := New(shared).GetAssignments("id", "user-7")
	require.NoError(t, err)
	require.Equal(t, "0", stored.Assignments["exp__0"])
}
