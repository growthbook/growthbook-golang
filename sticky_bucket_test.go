package growthbook

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Helper function to create a client with debug logging enabled
func newClientWithDebugLogs(ctx context.Context, opts ...ClientOption) (*Client, error) {
	// Create a logger that writes to stdout with debug level
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Add the logger option to the provided options
	allOpts := append([]ClientOption{WithLogger(logger)}, opts...)

	// Create and return the client
	return NewClient(ctx, allOpts...)
}

func TestInMemoryStickyBucketService(t *testing.T) {
	// Create a new in-memory service
	service := NewInMemoryStickyBucketService()

	// Test getting non-existent assignments
	doc, err := service.GetAssignments("userId", "123")
	require.NoError(t, err)
	require.Nil(t, doc)

	// Test saving a new assignment
	saveDoc := &StickyBucketAssignmentDoc{
		AttributeName:  "userId",
		AttributeValue: "123",
		Assignments:    map[string]string{"exp__0": "control"},
	}
	err = service.SaveAssignments(saveDoc)
	require.NoError(t, err)

	// Test retrieving the saved assignment
	doc, err = service.GetAssignments("userId", "123")
	require.NoError(t, err)
	require.NotNil(t, doc)
	require.Equal(t, "userId", doc.AttributeName)
	require.Equal(t, "123", doc.AttributeValue)
	require.Equal(t, map[string]string{"exp__0": "control"}, doc.Assignments)

	// Test retrieving all assignments
	allDocs, err := service.GetAllAssignments(map[string]string{"userId": "123"})
	require.NoError(t, err)
	require.Len(t, allDocs, 1)
	key := getKey("userId", "123")
	require.NotNil(t, allDocs[key])

	// Test destroying all assignments
	service.Destroy()
	doc, err = service.GetAssignments("userId", "123")
	require.NoError(t, err)
	require.Nil(t, doc)
}

func TestGetStickyBucketVariation(t *testing.T) {
	// Create a test service with pre-populated data
	service := NewInMemoryStickyBucketService()
	saveDoc := &StickyBucketAssignmentDoc{
		AttributeName:  "userId",
		AttributeValue: "123",
		Assignments: map[string]string{
			"exp1__0":    "control",
			"exp2__0":    "treatment",
			"old_exp__0": "old-variation",
		},
	}
	service.SaveAssignments(saveDoc)

	// Test meta information
	meta := []VariationMeta{
		{Key: "control"},
		{Key: "treatment"},
	}

	// Test case: Found existing variation
	result, err := GetStickyBucketVariation(
		"exp1",
		0,
		0,
		meta,
		service,
		"userId",
		"",
		map[string]string{"userId": "123"},
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, 0, result.Variation) // "control" is at index 0
	require.False(t, result.VersionIsBlocked)

	// Test case: Found different variation
	result, err = GetStickyBucketVariation(
		"exp2",
		0,
		0,
		meta,
		service,
		"userId",
		"",
		map[string]string{"userId": "123"},
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, 1, result.Variation) // "treatment" is at index 1
	require.False(t, result.VersionIsBlocked)

	// Test case: Not found
	result, err = GetStickyBucketVariation(
		"nonexistent",
		0,
		0,
		meta,
		service,
		"userId",
		"",
		map[string]string{"userId": "123"},
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, -1, result.Variation)
	require.False(t, result.VersionIsBlocked)

	// Test: version blocking
	result, err = GetStickyBucketVariation(
		"old_exp",
		1, // New version
		1, // Block users from version 0
		meta,
		service,
		"userId",
		"",
		map[string]string{"userId": "123"},
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, -1, result.Variation)
	require.True(t, result.VersionIsBlocked)
}

func TestStickyBucketSaveAssignment(t *testing.T) {
	// Create a test service
	service := NewInMemoryStickyBucketService()

	// Initial save
	err := SaveStickyBucketAssignment(
		"exp1",
		0,
		0,
		"control",
		service,
		"userId",
		"123",
		nil,
	)
	require.NoError(t, err)

	// Verify it was saved
	doc, err := service.GetAssignments("userId", "123")
	require.NoError(t, err)
	require.NotNil(t, doc)
	require.Equal(t, "control", doc.Assignments["exp1__0"])

	// Update the same experiment
	err = SaveStickyBucketAssignment(
		"exp1",
		0,
		1,
		"treatment",
		service,
		"userId",
		"123",
		nil,
	)
	require.NoError(t, err)

	// Verify it was updated
	doc, err = service.GetAssignments("userId", "123")
	require.NoError(t, err)
	require.NotNil(t, doc)
	require.Equal(t, "treatment", doc.Assignments["exp1__0"])

	// Add another experiment
	err = SaveStickyBucketAssignment(
		"exp2",
		0,
		0,
		"control2",
		service,
		"userId",
		"123",
		nil,
	)
	require.NoError(t, err)

	// Verify both are present
	doc, err = service.GetAssignments("userId", "123")
	require.NoError(t, err)
	require.NotNil(t, doc)
	require.Equal(t, "treatment", doc.Assignments["exp1__0"])
	require.Equal(t, "control2", doc.Assignments["exp2__0"])
}

// Common test helpers
func createTestExperiment(key string, weights []float64, metaKeys []string) *Experiment {
	variations := []FeatureValue{"control", "treatment"}
	meta := make([]VariationMeta, len(metaKeys))
	for i, key := range metaKeys {
		meta[i] = VariationMeta{Key: key}
	}

	return &Experiment{
		Key:        key,
		Variations: variations,
		Meta:       meta,
		Weights:    weights,
	}
}

func createTestClient(t *testing.T, ctx context.Context, service StickyBucketService) *Client {
	client, err := newClientWithDebugLogs(
		ctx,
		WithAttributes(Attributes{"id": "123"}),
		WithStickyBucketService(service),
	)
	require.NoError(t, err)
	return client
}

func TestStickyBucketInExperimentEvaluation(t *testing.T) {
	service := NewInMemoryStickyBucketService()
	ctx := context.TODO()

	// Basic experiment
	exp := createTestExperiment("test-exp", []float64{1.0, 0.0}, []string{"0", "1"})
	client := createTestClient(t, ctx, service)

	// First run
	result := client.RunExperiment(ctx, exp)
	require.True(t, result.InExperiment)
	require.Equal(t, 0, result.VariationId)

	// Verify sticky bucket
	doc, _ := service.GetAssignments("id", "123")
	require.Contains(t, doc.Assignments, "test-exp__0")

	// Second run
	result2 := client.RunExperiment(ctx, exp)
	require.True(t, result2.InExperiment)
	require.Equal(t, 0, result2.VariationId)
	require.True(t, result2.StickyBucketUsed)

	// Version blocking test
	expNew := createTestExperiment("test-exp", []float64{1.0, 0.0}, []string{"control-key", "treatment-key"})
	expNew.BucketVersion = 1
	expNew.MinBucketVersion = 1

	result3 := client.RunExperiment(ctx, expNew)
	require.False(t, result3.InExperiment)
	require.True(t, result3.StickyBucketUsed)
	require.Equal(t, 0, result3.VariationId)
}

func TestStickyBucketWithFallbackAttribute(t *testing.T) {
	service := NewInMemoryStickyBucketService()

	// Setup an assignment with a fallback attribute
	saveDoc := &StickyBucketAssignmentDoc{
		AttributeName:  "deviceId",
		AttributeValue: "device123",
		Assignments: map[string]string{
			"exp1__0": "fallback-variation",
		},
	}
	service.SaveAssignments(saveDoc)

	// Create experiment that uses userId with deviceId as fallback
	exp := &Experiment{
		Key:               "exp1",
		Variations:        []FeatureValue{"var1", "var2"},
		HashAttribute:     "userId",
		FallbackAttribute: "deviceId",
		Meta: []VariationMeta{
			{Key: "fallback-variation"},
			{Key: "other-variation"},
		},
	}

	// Create client with both userId and deviceId
	ctx := context.TODO()
	client, _ := NewClient(
		ctx,
		WithAttributes(Attributes{
			"userId":   "user123",
			"deviceId": "device123",
		}),
		WithStickyBucketService(service),
	)

	// Run experiment - should use the fallback assignment
	result := client.RunExperiment(ctx, exp)
	require.True(t, result.InExperiment)
	require.Equal(t, 0, result.VariationId) // fallback-variation is at index 0
	require.True(t, result.StickyBucketUsed)
}

func TestStickyBucketCaching(t *testing.T) {
	service := NewInMemoryStickyBucketService()
	mockCalls := 0
	mockService := &mockStickyBucketService{
		InMemoryStickyBucketService: service,
		onGetAssignments: func(name, value string) {
			mockCalls++
		},
	}

	ctx := context.TODO()
	client := createTestClient(t, ctx, mockService)

	// First experiment
	exp1 := createTestExperiment("cache-test", []float64{1.0, 0.0}, []string{"0", "1"})

	// First run should make 2 calls
	result1 := client.RunExperiment(ctx, exp1)
	require.Equal(t, 2, mockCalls, "First run should make 2 calls")
	require.True(t, result1.InExperiment)

	// Second run should use cache
	mockCalls = 0
	result2 := client.RunExperiment(ctx, exp1)
	require.True(t, result2.StickyBucketUsed)

	// Different experiment, same attribute
	mockCalls = 0
	exp2 := createTestExperiment("cache-test2", []float64{1.0, 0.0}, []string{"0", "1"})
	result3 := client.RunExperiment(ctx, exp2)
	require.Equal(t, 1, mockCalls, "New experiment should make 1 call")
	require.True(t, result3.InExperiment)

	// Verify cache contains both experiments
	cacheKey := getKey("id", "123")
	cache, ok := client.stickyBucketAssignments.get(cacheKey)
	require.True(t, ok)
	require.Len(t, cache.Assignments, 2, "Cache should contain both experiments")
}

// The assignments cache is shared between a client and all clients cloned
// from it, so concurrent EvalFeature calls must not race on it. Run with
// -race to catch regressions.
func TestStickyBucketConcurrentEvaluation(t *testing.T) {
	featuresJSON := `{
		"feat-a": {
			"defaultValue": "off",
			"rules": [{
				"key": "exp-a",
				"variations": ["control", "treatment"],
				"meta": [{"key": "0"}, {"key": "1"}],
				"hashAttribute": "id"
			}]
		},
		"feat-b": {
			"defaultValue": "off",
			"rules": [{
				"key": "exp-b",
				"variations": ["control", "treatment"],
				"meta": [{"key": "0"}, {"key": "1"}],
				"hashAttribute": "id"
			}]
		}
	}`

	ctx := context.Background()
	client, err := NewClient(ctx,
		WithJsonFeatures(featuresJSON),
		WithStickyBucketService(NewInMemoryStickyBucketService()),
	)
	require.NoError(t, err)

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// A small pool of user ids so goroutines both write fresh cache
			// entries and hit the same entries concurrently.
			child, err := client.WithAttributes(Attributes{"id": fmt.Sprintf("user-%d", i%4)})
			if err != nil {
				t.Error(err)
				return
			}
			for j := 0; j < 20; j++ {
				child.EvalFeature(ctx, "feat-a")
				child.EvalFeature(ctx, "feat-b")
			}
		}(i)
	}
	wg.Wait()
}

func TestStickyBucketCacheLRUEviction(t *testing.T) {
	doc := func(val string) *StickyBucketAssignmentDoc {
		return &StickyBucketAssignmentDoc{AttributeName: "id", AttributeValue: val}
	}

	cache := newStickyBucketCache(2)
	cache.set("a", doc("a"))
	cache.set("b", doc("b"))

	// Touch "a" so "b" becomes the least recently used entry.
	_, ok := cache.get("a")
	require.True(t, ok)

	cache.set("c", doc("c"))
	_, ok = cache.get("b")
	require.False(t, ok, "least recently used entry should be evicted")
	_, ok = cache.get("a")
	require.True(t, ok)
	_, ok = cache.get("c")
	require.True(t, ok)

	// Updating an existing key must not evict anything.
	cache.set("a", doc("a2"))
	got, ok := cache.get("c")
	require.True(t, ok)
	require.Equal(t, "c", got.AttributeValue)
	got, ok = cache.get("a")
	require.True(t, ok)
	require.Equal(t, "a2", got.AttributeValue)

	// size <= 0 means unbounded.
	unbounded := newStickyBucketCache(0)
	for i := 0; i < 100; i++ {
		unbounded.set(fmt.Sprintf("k%d", i), doc("v"))
	}
	for i := 0; i < 100; i++ {
		_, ok := unbounded.get(fmt.Sprintf("k%d", i))
		require.True(t, ok)
	}
}

// Eviction must be transparent: an evicted user's next evaluation re-fetches
// the assignment doc from the service and keeps the sticky variation.
func TestStickyBucketCacheEvictionRefetches(t *testing.T) {
	featuresJSON := `{
		"feat": {
			"defaultValue": "off",
			"rules": [{
				"key": "exp",
				"variations": ["control", "treatment"],
				"meta": [{"key": "0"}, {"key": "1"}],
				"hashAttribute": "id"
			}]
		}
	}`

	service := NewInMemoryStickyBucketService()
	ctx := context.Background()
	client, err := NewClient(ctx,
		WithJsonFeatures(featuresJSON),
		WithStickyBucketService(service),
		WithStickyBucketCacheSize(1),
	)
	require.NoError(t, err)

	c1, err := client.WithAttributes(Attributes{"id": "u1"})
	require.NoError(t, err)
	res1 := c1.EvalFeature(ctx, "feat")
	require.True(t, res1.ExperimentResult.InExperiment)

	// Evaluating a second user evicts u1 from the size-1 cache.
	c2, err := client.WithAttributes(Attributes{"id": "u2"})
	require.NoError(t, err)
	c2.EvalFeature(ctx, "feat")
	_, cached := client.stickyBucketAssignments.get(getKey("id", "u1"))
	require.False(t, cached, "u1 should be evicted from the size-1 cache")

	// u1's next evaluation re-fetches from the service: same variation,
	// via the stored sticky bucket rather than a fresh hash.
	res2 := c1.EvalFeature(ctx, "feat")
	require.True(t, res2.ExperimentResult.InExperiment)
	require.Equal(t, res1.ExperimentResult.VariationId, res2.ExperimentResult.VariationId)
	require.True(t, res2.ExperimentResult.StickyBucketUsed)
}

// Concurrent saves for the same user must merge rather than overwrite each
// other: saving is a read-modify-write cycle against the service, and without
// per-document serialization the last write drops the other's assignment.
//
// The gate in GetAssignments holds save-path reads open, so when saves are
// NOT serialized both deterministically read the same document version and
// the lost update reproduces. Once saves ARE serialized the second save
// cannot reach the read, so the timeout releases the gate and the saves run
// back to back — which is exactly the fixed behavior being asserted.
func TestStickyBucketOverlappingSavesMerge(t *testing.T) {
	inner := NewInMemoryStickyBucketService()
	readerArrived := make(chan struct{}, 2)
	release := make(chan struct{})
	svc := &mockStickyBucketService{
		InMemoryStickyBucketService: inner,
		onGetAssignments: func(_, _ string) {
			select {
			case readerArrived <- struct{}{}:
			default:
			}
			<-release
		},
	}
	go func() {
		timeout := time.After(200 * time.Millisecond)
		for i := 0; i < 2; i++ {
			select {
			case <-readerArrived:
			case <-timeout:
				close(release)
				return
			}
		}
		close(release)
	}()

	cache := newStickyBucketCache(defaultStickyBucketCacheSize)
	var wg sync.WaitGroup
	for _, save := range []struct{ expKey, variation string }{
		{"exp-a", "control"},
		{"exp-b", "treatment"},
	} {
		wg.Add(1)
		go func(expKey, variation string) {
			defer wg.Done()
			if err := saveStickyBucketAssignment(expKey, 0, 0, variation, svc, "id", "u1", cache); err != nil {
				t.Error(err)
			}
		}(save.expKey, save.variation)
	}
	wg.Wait()

	doc, err := inner.GetAssignments("id", "u1")
	require.NoError(t, err)
	require.NotNil(t, doc)
	require.Equal(t, "control", doc.Assignments["exp-a__0"], "assignment for exp-a was lost")
	require.Equal(t, "treatment", doc.Assignments["exp-b__0"], "assignment for exp-b was lost")

	// The shared cache must hold the merged document too.
	cached, ok := cache.get(getKey("id", "u1"))
	require.True(t, ok)
	require.Equal(t, "control", cached.Assignments["exp-a__0"])
	require.Equal(t, "treatment", cached.Assignments["exp-b__0"])
}

// mockStickyBucketService is a wrapper around InMemoryStickyBucketService that allows tracking calls
type mockStickyBucketService struct {
	*InMemoryStickyBucketService
	onGetAssignments func(attributeName, attributeValue string)
}

func (m *mockStickyBucketService) GetAssignments(name, value string) (*StickyBucketAssignmentDoc, error) {
	if m.onGetAssignments != nil {
		m.onGetAssignments(name, value)
	}
	return m.InMemoryStickyBucketService.GetAssignments(name, value)
}

func (m *mockStickyBucketService) SaveAssignments(doc *StickyBucketAssignmentDoc) error {
	return m.InMemoryStickyBucketService.SaveAssignments(doc)
}

func (m *mockStickyBucketService) GetAllAssignments(attributes map[string]string) (StickyBucketAssignments, error) {
	return m.InMemoryStickyBucketService.GetAllAssignments(attributes)
}

// A cache-miss read that raced with a save must not re-install its stale
// service snapshot after the save's fresher doc was evicted from the LRU —
// cache hits never re-consult the service, so a stale doc would stick and
// the user would be re-hashed (and the flip persisted by the next save).
func TestStickyBucketReadDoesNotReinstallStaleDoc(t *testing.T) {
	inner := NewInMemoryStickyBucketService()
	require.NoError(t, inner.SaveAssignments(&StickyBucketAssignmentDoc{
		AttributeName:  "id",
		AttributeValue: "u1",
		Assignments:    map[string]string{"other__0": "0"},
	}))

	// Gate the reader AFTER its service fetch, so it holds a stale snapshot
	// of the doc while a save merges and persists a fresher one. Only the
	// first caller parks (CAS), so the save's own service read passes.
	svc := &postReadGateService{
		InMemoryStickyBucketService: inner,
		entered:                     make(chan struct{}),
		release:                     make(chan struct{}),
	}
	svc.gate.Store(true)
	entered, release := svc.entered, svc.release

	cache := newStickyBucketCache(1)

	// Reader: cache miss -> service fetch, parked at the gate.
	readErr := make(chan error, 1)
	go func() {
		_, err := getStickyBucketAssignments(svc, "id", "", map[string]string{"id": "u1"}, cache)
		readErr <- err
	}()
	<-entered

	// Concurrent save merges a new assignment for the same user. With the
	// read-path doc lock it waits for the reader; without it, it completes
	// now and its cached doc is about to be evicted.
	saveErr := make(chan error, 1)
	go func() {
		saveErr <- saveStickyBucketAssignment("exp", 0, 1, "1", svc, "id", "u1", cache)
	}()
	savedEarly := false
	select {
	case err := <-saveErr:
		require.NoError(t, err)
		savedEarly = true
	case <-time.After(100 * time.Millisecond):
	}

	// Evict whatever the size-1 cache holds for u1.
	cache.set(getKey("id", "other"), &StickyBucketAssignmentDoc{
		AttributeName:  "id",
		AttributeValue: "other",
		Assignments:    map[string]string{},
	})

	close(release)
	require.NoError(t, <-readErr)
	if !savedEarly {
		require.NoError(t, <-saveErr)
	}

	// However the read and save interleave, a cached doc for u1 must not
	// hide the saved assignment: it either holds the merged doc or was
	// evicted (forcing a service re-fetch on the next evaluation).
	if cached, ok := cache.get(getKey("id", "u1")); ok {
		require.Equal(t, "1", cached.Assignments["exp__0"],
			"stale doc without the saved assignment was re-installed into the cache")
	}
}

// postReadGateService parks the first GetAssignments call for id||u1 after
// the read completes, simulating a reader stalled between its service fetch
// and its cache install.
type postReadGateService struct {
	*InMemoryStickyBucketService
	gate    atomic.Bool
	entered chan struct{}
	release chan struct{}
}

func (s *postReadGateService) GetAssignments(name, value string) (*StickyBucketAssignmentDoc, error) {
	doc, err := s.InMemoryStickyBucketService.GetAssignments(name, value)
	if name == "id" && value == "u1" && s.gate.CompareAndSwap(true, false) {
		close(s.entered)
		<-s.release
	}
	return doc, err
}

func TestRolloutRuleFallbackAttribute(t *testing.T) {
	// Mirrors JS evalFeature (sdk-js core.ts): rollout inclusion passes the
	// rule's fallbackAttribute to getHashAttribute only when a sticky bucket
	// service is configured and the rule doesn't disable sticky bucketing.
	ctx := context.TODO()
	coverage := 1.0
	newFeatures := func(disableStickyBucketing bool) FeatureMap {
		return FeatureMap{
			"feature": {
				DefaultValue: 0,
				Rules: []FeatureRule{{
					Force:                  1,
					Coverage:               &coverage,
					FallbackAttribute:      "deviceId",
					DisableStickyBucketing: disableStickyBucketing,
				}},
			},
		}
	}
	attrs := Attributes{"deviceId": "device123"} // no "id" attribute

	t.Run("fallback used with sticky bucket service", func(t *testing.T) {
		client, err := NewClient(ctx,
			WithAttributes(attrs),
			WithFeatures(newFeatures(false)),
			WithStickyBucketService(NewInMemoryStickyBucketService()),
		)
		require.NoError(t, err)
		res := client.EvalFeature(ctx, "feature")
		require.Equal(t, ForceResultSource, res.Source)
		require.Equal(t, 1, res.Value)
	})

	t.Run("fallback ignored without sticky bucket service", func(t *testing.T) {
		client, err := NewClient(ctx,
			WithAttributes(attrs),
			WithFeatures(newFeatures(false)),
		)
		require.NoError(t, err)
		res := client.EvalFeature(ctx, "feature")
		require.Equal(t, DefaultValueResultSource, res.Source)
	})

	t.Run("fallback ignored when rule disables sticky bucketing", func(t *testing.T) {
		client, err := NewClient(ctx,
			WithAttributes(attrs),
			WithFeatures(newFeatures(true)),
			WithStickyBucketService(NewInMemoryStickyBucketService()),
		)
		require.NoError(t, err)
		res := client.EvalFeature(ctx, "feature")
		require.Equal(t, DefaultValueResultSource, res.Source)
	})
}

func TestStickyBucketFalsyFallbackAttributeIgnored(t *testing.T) {
	// JS parity: a falsy fallback attribute value (0, "", false) must not be
	// used to look up sticky bucket assignment docs (sdk-js core.ts
	// getStickyBucketAssignments skips falsy fallback values).
	ctx := context.TODO()
	service := NewInMemoryStickyBucketService()
	require.NoError(t, service.SaveAssignments(&StickyBucketAssignmentDoc{
		AttributeName:  "anonymousId",
		AttributeValue: "0",
		Assignments:    map[string]string{"feature__0": "1"},
	}))

	client, err := NewClient(ctx,
		WithAttributes(Attributes{"id": "u1", "anonymousId": 0}),
		WithFeatures(FeatureMap{
			"feature": {
				DefaultValue: 0,
				Rules: []FeatureRule{{
					Variations:        []FeatureValue{0, 1},
					Meta:              []VariationMeta{{Key: "0"}, {Key: "1"}},
					FallbackAttribute: "anonymousId",
				}},
			},
		}),
		WithStickyBucketService(service),
	)
	require.NoError(t, err)

	res := client.EvalFeature(ctx, "feature")
	require.NotNil(t, res.ExperimentResult)
	require.False(t, res.ExperimentResult.StickyBucketUsed,
		"sticky doc keyed by falsy fallback value must not be applied")
}

// Steady state: once an assignment is saved and cached, repeat evaluations
// must not touch the sticky bucket service at all — JS/Python answer
// "unchanged?" from memory, and the save is skipped entirely when the
// variation came from the sticky bucket (evaluator step 13.5).
func TestStickyBucketSteadyStateNoServiceCalls(t *testing.T) {
	ctx := context.TODO()
	var calls atomic.Int32
	svc := &mockStickyBucketService{
		InMemoryStickyBucketService: NewInMemoryStickyBucketService(),
		onGetAssignments: func(_, _ string) {
			calls.Add(1)
		},
	}

	client, err := NewClient(ctx,
		WithAttributes(Attributes{"id": "steady-user"}),
		WithFeatures(FeatureMap{
			"feature": {
				DefaultValue: 0,
				Rules: []FeatureRule{{
					Variations: []FeatureValue{0, 1},
					Meta:       []VariationMeta{{Key: "0"}, {Key: "1"}},
				}},
			},
		}),
		WithStickyBucketService(svc),
		withSilentTestLogger(),
	)
	require.NoError(t, err)

	// First evaluation: read miss + save cycle both hit the service.
	res1 := client.EvalFeature(ctx, "feature")
	require.True(t, res1.InExperiment())
	require.Positive(t, calls.Load())

	// Steady state: everything answered from the client cache.
	calls.Store(0)
	res2 := client.EvalFeature(ctx, "feature")
	require.True(t, res2.InExperiment())
	require.True(t, res2.ExperimentResult.StickyBucketUsed)
	require.Equal(t, res1.Value, res2.Value)
	require.Equal(t, int32(0), calls.Load(),
		"steady-state evaluation must not call the sticky bucket service")
}

// A negative bucketVersion must read and write the same assignment key.
// Previously the read normalized -1 to 0 ("exp__0") while the save wrote
// "exp__-1", so the saved assignment was never found again.
func TestStickyBucketNegativeBucketVersionRoundTrip(t *testing.T) {
	ctx := context.TODO()
	service := NewInMemoryStickyBucketService()
	client, err := NewClient(ctx,
		WithAttributes(Attributes{"id": "neg-user"}),
		WithStickyBucketService(service),
		withSilentTestLogger(),
	)
	require.NoError(t, err)

	exp := &Experiment{
		Key:           "neg-exp",
		Variations:    []FeatureValue{"control", "treatment"},
		Meta:          []VariationMeta{{Key: "0"}, {Key: "1"}},
		BucketVersion: -1,
	}

	res1 := client.RunExperiment(ctx, exp)
	require.True(t, res1.InExperiment)

	doc, err := service.GetAssignments("id", "neg-user")
	require.NoError(t, err)
	require.NotNil(t, doc)
	require.Contains(t, doc.Assignments, "neg-exp__0",
		"negative bucketVersion must normalize to 0 in the saved key")

	res2 := client.RunExperiment(ctx, exp)
	require.True(t, res2.StickyBucketUsed,
		"second run must find the assignment saved by the first")
}
