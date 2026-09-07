// Package redis provides a Redis-backed [growthbook.StickyBucketService].
//
// The in-memory service that ships with the SDK keeps assignments inside one
// process, so a horizontally scaled fleet buckets the same user independently:
// a user can see different variations from different instances, and every
// restart re-buckets everyone. Storing assignments in Redis makes them shared
// and durable, which is what sticky bucketing needs to mean anything in
// production.
//
// The wire format matches the JavaScript SDK's RedisStickyBucketService — one
// JSON-encoded document per key `<prefix><attributeName>||<attributeValue>` —
// so with the default empty prefix a Go and a JS service can share one Redis
// and read each other's assignments.
//
// Usage:
//
//	rdb := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
//	client, err := growthbook.NewClient(ctx,
//		growthbook.WithClientKey(key),
//		growthbook.WithStickyBucketService(gbredis.New(rdb)),
//	)
package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	growthbook "github.com/growthbook/growthbook-golang"
	goredis "github.com/redis/go-redis/v9"
)

// keySeparator joins an attribute name and value into an assignment key. It is
// the separator the SDK itself uses internally and the one the JS SDK writes to
// Redis; changing it would strand every assignment already stored.
const keySeparator = "||"

// Client is the subset of a go-redis client this service uses. Depending on the
// three commands actually needed rather than on redis.Cmdable keeps the
// coupling small and makes the service easy to fake in tests; *redis.Client,
// *redis.ClusterClient and redis.UniversalClient all satisfy it.
type Client interface {
	Get(ctx context.Context, key string) *goredis.StringCmd
	Set(ctx context.Context, key string, value any, expiration time.Duration) *goredis.StatusCmd
	MGet(ctx context.Context, keys ...string) *goredis.SliceCmd
}

// StickyBucketService reads and writes sticky bucket assignments in Redis.
//
// It is safe for concurrent use as long as the underlying client is, which is
// true of every go-redis client. Assignments for different users live under
// different keys, so concurrent evaluations do not contend.
type StickyBucketService struct {
	client    Client
	keyPrefix string
	baseCtx   context.Context
	timeout   time.Duration
}

// Compile-time proof that this type still satisfies the SDK interface: if the
// interface gains a method, this line fails to build instead of some caller
// failing at the point of use.
var _ growthbook.StickyBucketService = (*StickyBucketService)(nil)

// Option configures a [StickyBucketService].
type Option func(*StickyBucketService)

// WithKeyPrefix namespaces every Redis key this service touches. The default is
// empty, which keeps keys byte-identical to the JS service so a Redis can be
// shared with it; set a prefix when the same Redis holds unrelated data and a
// bare `id||123` might collide.
//
// On Redis Cluster, MGet requires all keys to live in one hash slot. Use a hash
// tag to force that, e.g. WithKeyPrefix("{growthbook}:").
func WithKeyPrefix(prefix string) Option {
	return func(s *StickyBucketService) { s.keyPrefix = prefix }
}

// WithTimeout bounds each individual Redis call. Zero, the default, means no
// timeout beyond whatever the base context and the client itself impose.
func WithTimeout(timeout time.Duration) Option {
	return func(s *StickyBucketService) {
		if timeout > 0 {
			s.timeout = timeout
		}
	}
}

// WithContext sets the base context for Redis calls, letting a shutdown cancel
// in-flight commands. It defaults to [context.Background].
//
// A context normally travels as a function argument rather than living in a
// struct. It has to live here because [growthbook.StickyBucketService] takes no
// context — the SDK calls these methods from inside feature evaluation, which
// has no context to hand down.
func WithContext(ctx context.Context) Option {
	return func(s *StickyBucketService) {
		if ctx != nil {
			s.baseCtx = ctx
		}
	}
}

// New returns a service that stores assignments in the given Redis client.
func New(client Client, opts ...Option) *StickyBucketService {
	s := &StickyBucketService{
		client:  client,
		baseCtx: context.Background(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	return s
}

// GetAssignments returns the assignment document for one attribute, or nil when
// Redis holds none. A missing key is an ordinary miss and reports no error;
// only a genuine Redis failure or an unreadable document does.
func (s *StickyBucketService) GetAssignments(attributeName, attributeValue string) (*growthbook.StickyBucketAssignmentDoc, error) {
	if attributeName == "" || attributeValue == "" {
		return nil, nil
	}

	ctx, cancel := s.opContext()
	defer cancel()

	key := s.redisKey(attributeName, attributeValue)
	raw, err := s.client.Get(ctx, key).Bytes()
	switch {
	case errors.Is(err, goredis.Nil):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("growthbook redis: reading sticky bucket assignments at %q: %w", key, err)
	}
	return decodeDoc(key, raw)
}

// GetAllAssignments fetches the documents for several attributes in one MGET
// rather than a round-trip each. Attributes with no document are simply absent
// from the result. Keys in the returned map are `attributeName||attributeValue`,
// the form the SDK uses internally.
//
// The SDK does not currently call this — its evaluation path fetches documents
// one attribute at a time through GetAssignments — but it is part of the
// interface, and callers doing their own prefetch should get the batched
// round-trip rather than a loop.
func (s *StickyBucketService) GetAllAssignments(attributes map[string]string) (growthbook.StickyBucketAssignments, error) {
	docs := growthbook.StickyBucketAssignments{}
	if len(attributes) == 0 {
		return docs, nil
	}

	// Two parallel slices: what the SDK will key the result by, and what Redis
	// is asked for. They differ whenever a key prefix is configured.
	docKeys := make([]string, 0, len(attributes))
	redisKeys := make([]string, 0, len(attributes))
	for name, val := range attributes {
		if name == "" || val == "" {
			continue
		}
		docKeys = append(docKeys, docKey(name, val))
		redisKeys = append(redisKeys, s.redisKey(name, val))
	}
	if len(redisKeys) == 0 {
		return docs, nil
	}

	ctx, cancel := s.opContext()
	defer cancel()

	values, err := s.client.MGet(ctx, redisKeys...).Result()
	if err != nil {
		return nil, fmt.Errorf("growthbook redis: reading sticky bucket assignments: %w", err)
	}
	if len(values) != len(redisKeys) {
		return nil, fmt.Errorf("growthbook redis: MGET returned %d values for %d keys", len(values), len(redisKeys))
	}

	for i, v := range values {
		if v == nil {
			continue // no document for this attribute
		}
		raw, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("growthbook redis: sticky bucket key %q holds %T, want a string", redisKeys[i], v)
		}
		doc, err := decodeDoc(redisKeys[i], []byte(raw))
		if err != nil {
			return nil, err
		}
		if doc != nil {
			docs[docKeys[i]] = doc
		}
	}
	return docs, nil
}

// SaveAssignments writes a user's assignment document.
//
// The document is stored without an expiry, deliberately: an assignment has to
// outlive any single process and any TTL a cache would impose, or the user gets
// re-bucketed once it lapses and the bucketing stops being sticky.
func (s *StickyBucketService) SaveAssignments(doc *growthbook.StickyBucketAssignmentDoc) error {
	if doc == nil {
		return nil
	}
	if doc.AttributeName == "" || doc.AttributeValue == "" {
		return fmt.Errorf("growthbook redis: sticky bucket document needs both attributeName and attributeValue")
	}

	payload, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("growthbook redis: encoding sticky bucket document: %w", err)
	}

	ctx, cancel := s.opContext()
	defer cancel()

	key := s.redisKey(doc.AttributeName, doc.AttributeValue)
	if err := s.client.Set(ctx, key, payload, 0).Err(); err != nil {
		return fmt.Errorf("growthbook redis: writing sticky bucket assignments to %q: %w", key, err)
	}
	return nil
}

// opContext derives the context for a single Redis call from the base context
// and the optional per-call timeout.
func (s *StickyBucketService) opContext() (context.Context, context.CancelFunc) {
	if s.timeout > 0 {
		return context.WithTimeout(s.baseCtx, s.timeout)
	}
	return context.WithCancel(s.baseCtx)
}

// redisKey is the key stored in Redis, which is the SDK's document key with any
// configured prefix in front.
func (s *StickyBucketService) redisKey(attributeName, attributeValue string) string {
	return s.keyPrefix + docKey(attributeName, attributeValue)
}

// docKey is how the SDK identifies an assignment document.
func docKey(attributeName, attributeValue string) string {
	return attributeName + keySeparator + attributeValue
}

// decodeDoc parses a stored document, rejecting anything that is not one.
// Handing back a zero-valued document instead would look like "this user has no
// assignments yet" and quietly re-bucket them, so a key holding foreign data —
// easy to hit with the default empty prefix on a shared Redis — is an error.
func decodeDoc(key string, raw []byte) (*growthbook.StickyBucketAssignmentDoc, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var doc growthbook.StickyBucketAssignmentDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("growthbook redis: unreadable sticky bucket document at %q: %w", key, err)
	}
	if doc.AttributeName == "" {
		return nil, fmt.Errorf("growthbook redis: value at %q is not a sticky bucket document (no attributeName)", key)
	}
	return &doc, nil
}
