package growthbook

import (
	"container/list"
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

// stickyBucketCache abstracts the assignments cache so the client can guard
// its shared cache with a mutex while the exported helpers keep accepting a
// plain StickyBucketAssignments map.
type stickyBucketCache interface {
	get(key string) (*StickyBucketAssignmentDoc, bool)
	set(key string, doc *StickyBucketAssignmentDoc)
	setIfAbsent(key string, doc *StickyBucketAssignmentDoc)
	// lockDoc serializes read-modify-write save cycles for one document key.
	lockDoc(key string) (unlock func())
}

func (a StickyBucketAssignments) get(key string) (*StickyBucketAssignmentDoc, bool) {
	doc, ok := a[key]
	return doc, ok
}

func (a StickyBucketAssignments) set(key string, doc *StickyBucketAssignmentDoc) {
	a[key] = doc
}

func (a StickyBucketAssignments) setIfAbsent(key string, doc *StickyBucketAssignmentDoc) {
	if _, ok := a[key]; !ok {
		a[key] = doc
	}
}

// lockDoc is a no-op: the exported helpers taking a plain map remain
// unsynchronized, as they always were.
func (a StickyBucketAssignments) lockDoc(string) func() {
	return func() {}
}

// asStickyBucketCache converts a possibly-nil assignments map to a cache,
// keeping a nil map as a nil interface so cache presence checks work.
func asStickyBucketCache(assignments StickyBucketAssignments) stickyBucketCache {
	if assignments == nil {
		return nil
	}
	return assignments
}

// defaultStickyBucketCacheSize bounds the client's assignments cache. The
// cache holds one entry per attribute value evaluated, so a long-lived
// multi-user client would otherwise grow it forever. Evicting an entry only
// costs a re-fetch from the StickyBucketService.
const defaultStickyBucketCacheSize = 10_000

// lockedStickyBucketCache is the client's assignments cache: an LRU shared
// by reference between a client and its clones, so access is mutex-guarded.
type lockedStickyBucketCache struct {
	mu      sync.Mutex
	maxSize int // <= 0 disables eviction
	docs    map[string]*list.Element
	order   *list.List // front = most recently used
	// docLocks serializes save cycles per document key so concurrent saves
	// for the same user merge instead of overwriting each other. Key-scoped
	// so a save's backend I/O never blocks work on other documents.
	docLocks keyedMutex
}

type stickyBucketCacheEntry struct {
	key string
	doc *StickyBucketAssignmentDoc
}

func newStickyBucketCache(maxSize int) *lockedStickyBucketCache {
	return &lockedStickyBucketCache{
		maxSize: maxSize,
		docs:    make(map[string]*list.Element),
		order:   list.New(),
	}
}

func (c *lockedStickyBucketCache) get(key string) (*StickyBucketAssignmentDoc, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	elem, ok := c.docs[key]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(elem)
	return elem.Value.(*stickyBucketCacheEntry).doc, true
}

func (c *lockedStickyBucketCache) set(key string, doc *StickyBucketAssignmentDoc) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.insert(key, doc)
}

func (c *lockedStickyBucketCache) setIfAbsent(key string, doc *StickyBucketAssignmentDoc) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.docs[key]; !ok {
		c.insert(key, doc)
	}
}

// insert adds or updates an entry and evicts the least recently used one
// when over capacity. Callers must hold c.mu.
func (c *lockedStickyBucketCache) insert(key string, doc *StickyBucketAssignmentDoc) {
	if elem, ok := c.docs[key]; ok {
		elem.Value.(*stickyBucketCacheEntry).doc = doc
		c.order.MoveToFront(elem)
		return
	}
	c.docs[key] = c.order.PushFront(&stickyBucketCacheEntry{key: key, doc: doc})
	if c.maxSize > 0 && c.order.Len() > c.maxSize {
		oldest := c.order.Back()
		c.order.Remove(oldest)
		delete(c.docs, oldest.Value.(*stickyBucketCacheEntry).key)
	}
}

func (c *lockedStickyBucketCache) lockDoc(key string) func() {
	return c.docLocks.lock(key)
}

// keyedMutex provides a mutex per string key. A key's entry is removed once
// no goroutine holds or waits for it, so memory does not grow with the
// number of keys ever locked.
type keyedMutex struct {
	mu    sync.Mutex
	locks map[string]*keyedLock
}

type keyedLock struct {
	mu   sync.Mutex
	refs int
}

func (k *keyedMutex) lock(key string) (unlock func()) {
	k.mu.Lock()
	if k.locks == nil {
		k.locks = make(map[string]*keyedLock)
	}
	l := k.locks[key]
	if l == nil {
		l = &keyedLock{}
		k.locks[key] = l
	}
	l.refs++
	k.mu.Unlock()

	l.mu.Lock()
	return func() {
		l.mu.Unlock()
		k.mu.Lock()
		l.refs--
		if l.refs == 0 {
			delete(k.locks, key)
		}
		k.mu.Unlock()
	}
}

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
	cachedAssignments StickyBucketAssignments,
) (*StickyBucketResult, error) {
	return getStickyBucketVariation(experimentKey, bucketVersion, minBucketVersion,
		meta, service, hashAttribute, fallbackAttribute, attributes,
		asStickyBucketCache(cachedAssignments))
}

func getStickyBucketVariation(
	experimentKey string,
	bucketVersion int,
	minBucketVersion int,
	meta []VariationMeta,
	service StickyBucketService,
	hashAttribute string,
	fallbackAttribute string,
	attributes map[string]string,
	cache stickyBucketCache,
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
	assignments, err := getStickyBucketAssignments(service, hashAttribute, fallbackAttribute, attributes, cache)
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
	cache stickyBucketCache,
) (map[string]string, error) {
	merged := make(map[string]string)

	if service == nil {
		return merged, nil
	}

	// Track which attributes we need to fetch from the service
	attributesToFetch := make(map[string]string)

	// Get the hash values
	hashValue, hasHash := attributes[hashAttribute]
	if hasHash {
		// Check if we have this in the cache first
		hashKey := getKey(hashAttribute, hashValue)
		if cache != nil {
			if doc, ok := cache.get(hashKey); ok && doc != nil {
				// Use cached assignments
				for k, v := range doc.Assignments {
					merged[k] = v
				}
			} else {
				// Need to fetch
				attributesToFetch[hashAttribute] = hashValue
			}
		} else {
			// No cache, need to fetch
			attributesToFetch[hashAttribute] = hashValue
		}
	}

	// Check fallback attribute
	if fallbackAttribute != "" && fallbackAttribute != hashAttribute {
		fallbackValue, hasFallback := attributes[fallbackAttribute]
		if hasFallback {
			// Check if we have this in the cache first
			fallbackKey := getKey(fallbackAttribute, fallbackValue)
			if cache != nil {
				if doc, ok := cache.get(fallbackKey); ok && doc != nil {
					// Use cached assignments, but don't overwrite existing ones
					for k, v := range doc.Assignments {
						if _, exists := merged[k]; !exists {
							merged[k] = v
						}
					}
				} else {
					// Need to fetch
					attributesToFetch[fallbackAttribute] = fallbackValue
				}
			} else {
				// No cache, need to fetch
				attributesToFetch[fallbackAttribute] = fallbackValue
			}
		}
	}

	// If we need to fetch anything from the service
	if len(attributesToFetch) > 0 {
		for attrName, attrValue := range attributesToFetch {
			doc, err := service.GetAssignments(attrName, attrValue)
			if err != nil {
				return merged, err
			}

			if doc != nil {
				// Store in merged assignments
				isPrimary := attrName == hashAttribute
				for k, v := range doc.Assignments {
					// For primary attribute, always use the value
					// For fallback, only use if not already set
					exists := false
					if !isPrimary {
						_, exists = merged[k]
					}
					if isPrimary || !exists {
						merged[k] = v
					}
				}

				// Update the cache if provided. setIfAbsent: a concurrent
				// save may have already cached a doc merged from fresher
				// service state; don't clobber it with this earlier read.
				if cache != nil {
					key := getKey(attrName, attrValue)
					cache.setIfAbsent(key, doc)
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
	cachedAssignments StickyBucketAssignments,
) error {
	return saveStickyBucketAssignment(experimentKey, bucketVersion, variationID,
		variationKey, service, attributeName, attributeValue,
		asStickyBucketCache(cachedAssignments))
}

func saveStickyBucketAssignment(
	experimentKey string,
	bucketVersion int,
	variationID int,
	variationKey string,
	service StickyBucketService,
	attributeName string,
	attributeValue string,
	cache stickyBucketCache,
) error {
	if service == nil || attributeName == "" || attributeValue == "" {
		return nil
	}

	// Saving is a read-modify-write cycle: serialize it per document so
	// concurrent saves for the same user merge instead of the last write
	// dropping the other's assignment.
	if cache != nil {
		unlock := cache.lockDoc(getKey(attributeName, attributeValue))
		defer unlock()
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
		// Update cache if provided
		if cache != nil {
			cache.set(data.Key, data.Doc)
		}
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
		Key:     getKey(attributeName, attributeValue),
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

	// If changes detected, build a new doc with merged assignments instead of
	// mutating the existing one: docs held by the service or the client cache
	// may be read concurrently by other evaluations.
	if result.Changed {
		mergedAssignments := make(map[string]string, len(doc.Assignments)+len(assignments))
		for k, v := range doc.Assignments {
			mergedAssignments[k] = v
		}
		for k, v := range assignments {
			mergedAssignments[k] = v
		}

		doc = &StickyBucketAssignmentDoc{
			AttributeName:  attributeName,
			AttributeValue: attributeValue,
			Assignments:    mergedAssignments,
		}
	}

	result.Doc = doc
	return result
}
