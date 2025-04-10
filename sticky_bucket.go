package growthbook

import (
	"fmt"
	"sync"
)

// StickyBucketAssignmentDoc represents a document storing assignment data
type StickyBucketAssignmentDoc struct {
	AttributeName  string            `json:"attributeName"`
	AttributeValue string            `json:"attributeValue"`
	Assignments    map[string]string `json:"assignments"`
}

// StickyBucketAssignments is a map of keys to assignment documents
type StickyBucketAssignments map[string]*StickyBucketAssignmentDoc

// StickyBucketService defines operations for storing and retrieving sticky bucket assignments
type StickyBucketService interface {
	GetAssignments(attributeName string, attributeValue string) (*StickyBucketAssignmentDoc, error)
	SaveAssignments(doc *StickyBucketAssignmentDoc) error
	GetAllAssignments(attributes map[string]string) (StickyBucketAssignments, error)
}

// StickyBucketResult holds the result of a sticky bucket lookup
type StickyBucketResult struct {
	Variation        int
	VersionIsBlocked bool
}

// InMemoryStickyBucketService provides a simple in-memory implementation of StickyBucketService
type InMemoryStickyBucketService struct {
	mu   sync.RWMutex
	docs map[string]*StickyBucketAssignmentDoc
}

// NewInMemoryStickyBucketService creates a new in-memory sticky bucket service
func NewInMemoryStickyBucketService() *InMemoryStickyBucketService {
	return &InMemoryStickyBucketService{
		docs: make(map[string]*StickyBucketAssignmentDoc),
	}
}

// GetKey generates a key for storing sticky bucket documents
func getKey(attributeName, attributeValue string) string {
	return attributeName + "||" + attributeValue
}

// GetAssignments retrieves assignments for a specific attribute
func (s *InMemoryStickyBucketService) GetAssignments(attributeName, attributeValue string) (*StickyBucketAssignmentDoc, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := getKey(attributeName, attributeValue)
	doc, ok := s.docs[key]
	if !ok {
		return nil, nil // Not found, but not an error
	}
	return doc, nil
}

// SaveAssignments stores assignments for a specific attribute
func (s *InMemoryStickyBucketService) SaveAssignments(doc *StickyBucketAssignmentDoc) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := getKey(doc.AttributeName, doc.AttributeValue)
	s.docs[key] = doc
	return nil
}

// GetAllAssignments retrieves all assignments for the provided attributes
func (s *InMemoryStickyBucketService) GetAllAssignments(attributes map[string]string) (StickyBucketAssignments, error) {
	result := make(StickyBucketAssignments)

	for attributeName, attributeValue := range attributes {
		doc, err := s.GetAssignments(attributeName, attributeValue)
		if err != nil {
			return nil, err
		}

		if doc != nil {
			key := getKey(attributeName, attributeValue)
			result[key] = doc
		}
	}

	return result, nil
}

// Destroy clears all stored assignments
func (s *InMemoryStickyBucketService) Destroy() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.docs = make(map[string]*StickyBucketAssignmentDoc)
}

// Helper functions for sticky bucketing

// getStickyBucketExperimentKey generates a key for storing experiment assignments
func getStickyBucketExperimentKey(experimentKey string, bucketVersion int) string {
	return fmt.Sprintf("%s__%d", experimentKey, bucketVersion)
}

// isVersionBlocked determines if a user should be excluded from newer versions of an experiment
func isVersionBlocked(assignments map[string]string, experimentKey string, minBucketVersion int) bool {
	if minBucketVersion <= 0 {
		return false
	}

	// Check if user was in any version from 0 to minBucketVersion-1
	for v := 0; v < minBucketVersion; v++ {
		key := getStickyBucketExperimentKey(experimentKey, v)
		if _, exists := assignments[key]; exists {
			return true
		}
	}

	return false
}

// GetStickyBucketVariation retrieves an existing sticky bucket assignment
func GetStickyBucketVariation(
	experimentKey string,
	bucketVersion int,
	minBucketVersion int,
	meta []VariationMeta,
	service StickyBucketService,
	hashAttribute string,
	fallbackAttribute string,
	attributes map[string]string,
) (*StickyBucketResult, error) {
	result := &StickyBucketResult{
		Variation:        -1,
		VersionIsBlocked: false,
	}

	// Default versions to 0 if not set
	if bucketVersion < 0 {
		bucketVersion = 0
	}
	if minBucketVersion < 0 {
		minBucketVersion = 0
	}

	// Get the experiment version key
	experimentVersionKey := getStickyBucketExperimentKey(experimentKey, bucketVersion)

	// Get assignments from both primary and fallback attributes
	assignments, err := getStickyBucketAssignments(service, hashAttribute, fallbackAttribute, attributes)
	if err != nil {
		return result, err
	}

	// Check if version is blocked
	if isVersionBlocked(assignments, experimentKey, minBucketVersion) {
		result.VersionIsBlocked = true
		return result, nil
	}

	// Check if there's an existing assignment for this experiment version
	variationKey, exists := assignments[experimentVersionKey]
	if !exists {
		return result, nil
	}

	// Find the variation index by key in the meta information
	for i, m := range meta {
		if m.Key == variationKey {
			result.Variation = i
			return result, nil
		}
	}

	// If we found a key but couldn't match it to meta, still return not found
	return result, nil
}

// getStickyBucketAssignments retrieves assignments for both primary and fallback attributes
func getStickyBucketAssignments(
	service StickyBucketService,
	hashAttribute string,
	fallbackAttribute string,
	attributes map[string]string,
) (map[string]string, error) {
	merged := make(map[string]string)

	if service == nil {
		return merged, nil
	}

	// Get the hash values
	hashValue, hasHash := attributes[hashAttribute]

	// Check primary attribute
	if hasHash {
		doc, err := service.GetAssignments(hashAttribute, hashValue)
		if err != nil {
			return merged, err
		}

		if doc != nil {
			// Copy assignments
			for k, v := range doc.Assignments {
				merged[k] = v
			}
		}
	}

	// Check fallback attribute
	if fallbackAttribute != "" && fallbackAttribute != hashAttribute {
		fallbackValue, hasFallback := attributes[fallbackAttribute]
		if hasFallback {
			doc, err := service.GetAssignments(fallbackAttribute, fallbackValue)
			if err != nil {
				return merged, err
			}

			if doc != nil {
				// Merge fallback assignments, but don't overwrite existing ones
				for k, v := range doc.Assignments {
					if _, exists := merged[k]; !exists {
						merged[k] = v
					}
				}
			}
		}
	}

	return merged, nil
}

// SaveStickyBucketAssignment saves a sticky bucket assignment
func SaveStickyBucketAssignment(
	experimentKey string,
	bucketVersion int,
	variationID int,
	variationKey string,
	service StickyBucketService,
	attributeName string,
	attributeValue string,
) error {
	if service == nil || attributeName == "" || attributeValue == "" {
		return nil
	}

	// Create assignment map with the experiment key and variation key
	assignments := make(map[string]string)
	experimentVersionKey := getStickyBucketExperimentKey(experimentKey, bucketVersion)
	assignments[experimentVersionKey] = variationKey

	// Generate the sticky bucket assignment document
	data := GenerateStickyBucketAssignmentDoc(
		attributeName,
		attributeValue,
		assignments,
		service,
	)

	// Only save if a change was detected
	if data.Doc != nil && data.Changed {
		return service.SaveAssignments(data.Doc)
	}

	return nil
}

// StickyBucketAssignmentData is used when generating sticky bucket assignments
type StickyBucketAssignmentData struct {
	Key     string
	Doc     *StickyBucketAssignmentDoc
	Changed bool
}

// GenerateStickyBucketAssignmentDoc creates or updates a sticky bucket assignment document
func GenerateStickyBucketAssignmentDoc(
	attributeName string,
	attributeValue string,
	assignments map[string]string,
	service StickyBucketService,
) *StickyBucketAssignmentData {
	result := &StickyBucketAssignmentData{
		Key:     attributeName + "||" + attributeValue,
		Changed: false,
	}

	if service == nil {
		return result
	}

	// Get existing assignment document
	doc, err := service.GetAssignments(attributeName, attributeValue)
	if err != nil {
		return result
	}

	// Create a new document if none exists
	if doc == nil {
		doc = &StickyBucketAssignmentDoc{
			AttributeName:  attributeName,
			AttributeValue: attributeValue,
			Assignments:    make(map[string]string),
		}
		result.Changed = true
	}

	// Check if there are changes by comparing assignment values
	for k, v := range assignments {
		if existingValue, ok := doc.Assignments[k]; !ok || existingValue != v {
			// This is either a new assignment or the value has changed
			result.Changed = true
			break
		}
	}

	// If changes detected, create merged assignments
	if result.Changed {
		// Create a copy of existing assignments
		mergedAssignments := make(map[string]string)
		for k, v := range doc.Assignments {
			mergedAssignments[k] = v
		}

		// Add or update with new assignments
		for k, v := range assignments {
			mergedAssignments[k] = v
		}

		doc.Assignments = mergedAssignments
	}

	result.Doc = doc
	return result
}
