package growthbook

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"

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

func TestStickyBucketInExperimentEvaluation(t *testing.T) {
	// Setup service with existing assignment
	service := NewInMemoryStickyBucketService()

	// Create experiment with fixed variations
	exp := &Experiment{
		Key:        "test-exp",
		Variations: []FeatureValue{"control", "treatment"},
		Meta: []VariationMeta{
			{Key: "0"},
			{Key: "1"},
		},
		// Force a specific variation by setting weights
		Weights: []float64{1.0, 0.0}, // 100% chance of variation 0
	}

	// Create a context with the sticky bucket service
	ctx := context.TODO()
	client, _ := newClientWithDebugLogs(
		ctx,
		WithAttributes(Attributes{"id": "123"}),
		WithStickyBucketService(service),
	)

	// First run - should assign variation 0 due to weights
	result := client.RunExperiment(ctx, exp)
	require.True(t, result.InExperiment)
	require.Equal(t, 0, result.VariationId) // Should get the first variation due to weights

	// Verify it was saved in the sticky bucket service
	doc, _ := service.GetAssignments("id", "123")
	require.NotNil(t, doc)
	require.Contains(t, doc.Assignments, "test-exp__0")

	// Run again - should get the same variation from sticky bucket
	result2 := client.RunExperiment(ctx, exp)
	require.True(t, result2.InExperiment)
	require.Equal(t, 0, result2.VariationId) // Should get the first variation
	require.True(t, result2.StickyBucketUsed)

	// Test: version blocking with a new version
	expNew := &Experiment{
		Key:        "test-exp",
		Variations: []FeatureValue{"control", "treatment"},
		Meta: []VariationMeta{
			{Key: "control-key"},
			{Key: "treatment-key"},
		},
		BucketVersion:    1,
		MinBucketVersion: 1,
		Weights:          []float64{1.0, 0.0}, // 100% chance of variation 0
	}

	// Should get a new variation because old version is blocked
	result3 := client.RunExperiment(ctx, expNew)
	fmt.Printf("[TEST] HashAttribute: %s\n", result3.HashAttribute)
	require.False(t, result3.InExperiment)
	require.True(t, result3.StickyBucketUsed)
	require.Equal(t, 0, result3.VariationId) // Should get the first variation due to weights

	// Verify the new version was saved
	doc, _ = service.GetAssignments("id", "123")
	require.NotNil(t, doc)
	require.NotContains(t, doc.Assignments, "test-exp__1")
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
	// Create a mock service wrapper to count calls
	callDetails := []string{}
	mockCalls := 0
	service := NewInMemoryStickyBucketService()
	mockService := &mockStickyBucketService{
		service: service,
		onGetAssignments: func(name, value string) {
			mockCalls++
			callDetails = append(callDetails, fmt.Sprintf("GetAssignments(%s, %s)", name, value))
		},
	}

	// Create experiment with fixed weights
	exp := &Experiment{
		Key:        "cache-test",
		Variations: []FeatureValue{"control", "treatment"},
		Meta:       []VariationMeta{{Key: "0"}, {Key: "1"}}, // Use numeric keys to match IDs
		Weights:    []float64{1.0, 0.0},                     // 100% chance of variation 0
	}

	// Create client with the sticky bucket service and initialize cache map
	ctx := context.TODO()
	client, _ := newClientWithDebugLogs(
		ctx,
		WithAttributes(Attributes{"id": "123"}),
		WithStickyBucketService(mockService),
	)

	// First run - checks for existing assignment and creates one
	result1 := client.RunExperiment(ctx, exp)
	require.True(t, result1.InExperiment)
	require.Equal(t, 0, result1.VariationId) // First variation due to weights

	// The first run always requires 2 GetAssignments calls:
	// 1) empty cache lookup
	// 2) save new assignment
	firstRunCalls := mockCalls
	require.Equal(t, 2, firstRunCalls, "First run should make exactly 2 GetAssignments calls")

	// Reset counter for second run
	mockCalls = 0
	callDetails = []string{}

	// Get the current cache state
	cacheKey := getKey("id", "123")
	beforeCache := client.stickyBucketAssignments[cacheKey]
	require.NotNil(t, beforeCache)
	require.Contains(t, beforeCache.Assignments, "cache-test__0")
	require.Equal(t, 1, len(beforeCache.Assignments), "Cache should have only one experiment")

	// Second run of the SAME experiment - should use client's cached assignments
	result2 := client.RunExperiment(ctx, exp)
	require.True(t, result2.InExperiment)
	require.Equal(t, 0, result2.VariationId)
	require.True(t, result2.StickyBucketUsed, "Should use sticky bucket on second run")
	// Second run should make no GetAssignments calls - complete cache hit
	require.Equal(t, 0, mockCalls, "Second run should make no calls to GetAssignments")
	require.Equal(t, 1, len(beforeCache.Assignments), "Cache should have only one experiment")

	// Reset counter
	mockCalls = 0
	callDetails = []string{}

	// Create a new experiment with the same hash attribute
	exp2 := &Experiment{
		Key:           "cache-test2",
		Variations:    []FeatureValue{"a", "b"},
		Meta:          []VariationMeta{{Key: "0"}, {Key: "1"}}, // Use numeric keys to match IDs
		HashAttribute: "id",
		Weights:       []float64{1.0, 0.0}, // 100% chance of variation 0
	}

	// first exp2 run but with SAME attribute
	result3 := client.RunExperiment(ctx, exp2)
	require.True(t, result3.InExperiment)
	require.Equal(t, 0, result3.VariationId)

	// The third run (different experiment, same attribute) makes exactly one call to GetAssignments
	require.Equal(t, 1, mockCalls, "Different experiment but same attribute should make exactly 1 call")

	// Get the updated cache
	afterCache := client.stickyBucketAssignments[cacheKey]
	require.NotNil(t, afterCache)
	require.Contains(t, afterCache.Assignments, "cache-test2__0", "Should now have second experiment")
	require.Equal(t, 2, len(afterCache.Assignments), "Cache should have both experiments")
}

// mockStickyBucketService is a wrapper around InMemoryStickyBucketService that allows tracking calls
type mockStickyBucketService struct {
	service          *InMemoryStickyBucketService
	onGetAssignments func(attributeName, attributeValue string)
}

func (m *mockStickyBucketService) GetAssignments(attributeName, attributeValue string) (*StickyBucketAssignmentDoc, error) {
	if m.onGetAssignments != nil {
		m.onGetAssignments(attributeName, attributeValue)
	}
	return m.service.GetAssignments(attributeName, attributeValue)
}

func (m *mockStickyBucketService) SaveAssignments(doc *StickyBucketAssignmentDoc) error {
	return m.service.SaveAssignments(doc)
}

func (m *mockStickyBucketService) GetAllAssignments(attributes map[string]string) (StickyBucketAssignments, error) {
	return m.service.GetAllAssignments(attributes)
}
