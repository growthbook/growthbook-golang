package growthbook

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/growthbook/growthbook-golang/internal/condition"
	"github.com/growthbook/growthbook-golang/internal/value"
)

// Remote saved groups (growthbook/growthbook#7277). A remote saved group keeps
// its members outside the payload, which only says which attribute the group
// is about: {"type": "remote", "attributeKey": ...}. The app resolves which of
// them a user is in, and conditions read the ids from the reserved
// __gb_remoteGroupIds attribute.

// RemoteSavedGroup is a remote saved group from the payload, as a
// SavedGroupResolver gets it.
type RemoteSavedGroup struct {
	ID           string `json:"id"`
	AttributeKey string `json:"attributeKey"`
	// ValuesFrom is set on a derived group: the remote group whose ids it
	// checks against its own attribute
	ValuesFrom string `json:"valuesFrom,omitempty"`
}

// ErrRemoteSavedGroupsChanged is returned by AddRemoteSavedGroups when the
// payload's remote saved groups kept changing while they were looked up, so no
// lookup covers the groups the client evaluates against.
var ErrRemoteSavedGroupsChanged = errors.New("growthbook: remote saved groups changed during every lookup")

// maxRemoteSavedGroupsLookups bounds how often AddRemoteSavedGroups looks up
// again when the payload changes during a lookup
const maxRemoteSavedGroupsLookups = 3

// SavedGroupResolver looks up which of groups a user is in. attributes holds
// only the attributes the groups use, keyed by AttributeKey (which can be a
// dot-separated path). It returns the ids of the groups the user is in.
type SavedGroupResolver func(ctx context.Context, attributes Attributes, groups []RemoteSavedGroup) ([]string, error)

// WithRemoteGroupIds sets the remote saved groups the user is in. Conditions
// read them as the __gb_remoteGroupIds attribute, over any value the
// attributes set for it.
//
// Leave them nil while the groups are unresolved: rules that use remote groups
// then fail, including "not in group". Don't pass an empty slice for that: it
// means "in no group", so "not in group" rules match.
func WithRemoteGroupIds(ids []string) ClientOption {
	return func(c *Client) error {
		c.remoteGroupIDs = slices.Clone(ids)
		return nil
	}
}

// WithRemoteGroupIds creates child client instance with the remote saved
// groups the user is in (see the WithRemoteGroupIds option).
func (c *Client) WithRemoteGroupIds(ids []string) (*Client, error) {
	return c.cloneWith(WithRemoteGroupIds(ids))
}

// RemoteGroupIds returns the remote saved groups the user is in, or nil while
// they are unresolved.
func (c *Client) RemoteGroupIds() []string {
	return slices.Clone(c.remoteGroupIDs)
}

// AddRemoteSavedGroups looks up the user's remote saved groups with resolver
// and returns a child client with them as its remote group ids.
//
// When the payload has no remote groups it returns the client unchanged. When
// the resolver fails, or the payload's remote groups change during every
// lookup (ErrRemoteSavedGroupsChanged), it returns the client unchanged and the
// error, so the client stays usable and rules using remote groups fail closed.
//
// Requires a saved group references v2 payload for the group metadata. With
// inline/v1 payloads, resolve membership in the app and use WithRemoteGroupIds.
func (c *Client) AddRemoteSavedGroups(ctx context.Context, resolver SavedGroupResolver) (*Client, error) {
	// No remote entries doesn't mean no remote rules: payloads for SDKs without
	// remote support check __gb_remoteGroupIds directly. Leaving it unset makes
	// those rules fail closed; [] would let every "not in group" rule match.
	groups, version := c.data.getRemoteGroups()
	for lookup := 0; lookup < maxRemoteSavedGroupsLookups; lookup++ {
		if len(groups) == 0 {
			return c, nil
		}
		ids, err := c.resolveRemoteGroupIDs(ctx, groups, resolver)
		if err != nil {
			return c, err
		}
		// A payload update during the lookup can add groups the result doesn't
		// cover, and a missing group would pass "not in group" rules. Keep the
		// result only if the remote groups are still the ones it looked up.
		current, currentVersion := c.data.getRemoteGroups()
		if currentVersion == version || slices.Equal(current, groups) {
			return c.cloneWith(WithRemoteGroupIds(ids))
		}
		groups, version = current, currentVersion
	}
	return c, ErrRemoteSavedGroupsChanged
}

func (c *Client) resolveRemoteGroupIDs(ctx context.Context, groups []condition.RemoteGroup, resolver SavedGroupResolver) ([]string, error) {
	attributes := Attributes{}
	remote := make([]RemoteSavedGroup, len(groups))
	for i, group := range groups {
		remote[i] = RemoteSavedGroup{ID: group.ID, AttributeKey: group.AttributeKey, ValuesFrom: group.ValuesFrom}
		v := value.Path(c.attributes, strings.Split(group.AttributeKey, ".")...)
		if !value.IsNull(v) {
			attributes[group.AttributeKey] = value.ToAny(v)
		}
	}
	ids, err := resolver(ctx, attributes, remote)
	if err != nil {
		return nil, fmt.Errorf("growthbook: resolving remote saved groups: %w", err)
	}
	if ids == nil {
		// nil would mean unresolved
		ids = []string{}
	}
	return ids, nil
}

// conditionAttributes returns the attributes targeting conditions see: the
// client's, with its remote group ids (when set) as __gb_remoteGroupIds. A
// copy, so the client's attributes are left as they were.
func (c *Client) conditionAttributes() value.ObjValue {
	if c.remoteGroupIDs == nil {
		return c.attributes
	}
	attrs := maps.Clone(c.attributes)
	if attrs == nil {
		attrs = value.ObjValue{}
	}
	ids := make([]any, len(c.remoteGroupIDs))
	for i, id := range c.remoteGroupIDs {
		ids[i] = id
	}
	attrs[condition.RemoteGroupIDsAttribute] = value.New(ids)
	return attrs
}

// warnIfRemoteGroupIDsMissing logs once, for the client and its children,
// when the payload has remote saved groups but the user has no remote group
// ids, since every rule using them will then fail. Logs no attribute values.
func (c *Client) warnIfRemoteGroupIDsMissing(groups []condition.RemoteGroup) {
	if len(groups) == 0 || c.remoteGroupIDs != nil || c.data.warnedMissingRemoteGroupIDs.Load() {
		return
	}
	if _, ok := c.attributes[condition.RemoteGroupIDsAttribute].(value.ArrValue); ok {
		return
	}
	if c.data.warnedMissingRemoteGroupIDs.CompareAndSwap(false, true) && c.logger != nil {
		c.logger.Warn("The payload has remote saved groups, but the user has no remote group ids. " +
			"Rules using them won't match. See AddRemoteSavedGroups.")
	}
}

// RedisSetClient is the one Redis command RedisResolver needs. Wrap your
// client, e.g. for go-redis:
//
//	type redisSets struct{ rdb *redis.Client }
//
//	func (r redisSets) SMembers(ctx context.Context, key string) ([]string, error) {
//		return r.rdb.SMembers(ctx, key).Result()
//	}
type RedisSetClient interface {
	SMembers(ctx context.Context, key string) ([]string, error)
}

// RedisResolver is a SavedGroupResolver for remote groups loaded into Redis by
// GrowthBook's loader: one SMEMBERS gb:member:<attribute>:<value> per
// attribute value.
//
// A derived group checks its own attribute's value in its source group's sets:
// grp_vip:parent_id looks up gb:member:<grp_vip's attribute>:<parent_id> and
// matches when grp_vip comes back.
func RedisResolver(redis RedisSetClient) SavedGroupResolver {
	return func(ctx context.Context, attributes Attributes, groups []RemoteSavedGroup) ([]string, error) {
		// For each group and value: the Redis key, and the id that means a match
		type check struct{ id, key, matchID string }
		var checks []check
		for _, group := range groups {
			source := group
			if group.ValuesFrom != "" {
				i := slices.IndexFunc(groups, func(g RemoteSavedGroup) bool { return g.ID == group.ValuesFrom })
				if i < 0 {
					continue
				}
				source = groups[i]
			}
			for _, v := range redisLookupValues(attributes[group.AttributeKey]) {
				checks = append(checks, check{group.ID, "gb:member:" + source.AttributeKey + ":" + v, source.ID})
			}
		}
		members := map[string][]string{}
		for _, ch := range checks {
			if _, done := members[ch.key]; done {
				continue
			}
			set, err := redis.SMembers(ctx, ch.key)
			if err != nil {
				return nil, err
			}
			members[ch.key] = set
		}
		// Checking for the expected id, not any id in the set, also keeps keys
		// that collide when attributes or values contain ":" (`a:b` + `c` and
		// `a` + `b:c`) from mixing up groups.
		ids := []string{}
		for _, ch := range checks {
			if slices.Contains(members[ch.key], ch.matchID) && !slices.Contains(ids, ch.id) {
				ids = append(ids, ch.id)
			}
		}
		return ids, nil
	}
}

// redisLookupValues returns the values to look up for an attribute: each item
// of a slice, strings as they are and numbers as JS String() formats them,
// the form the loader stores them in.
func redisLookupValues(attr any) []string {
	v := value.New(attr)
	items := []value.Value{v}
	if arr, ok := v.(value.ArrValue); ok {
		items = arr
	}
	var out []string
	for _, item := range items {
		switch item := item.(type) {
		case value.StrValue:
			out = append(out, string(item))
		case value.NumValue:
			if n := float64(item); !math.IsNaN(n) && !math.IsInf(n, 0) {
				out = append(out, jsNumberString(n))
			}
		}
	}
	return out
}

// jsNumberString formats a finite number like JS String(number): 2.0 is "2",
// 0.00001 is "0.00001" and 1e21 is "1e+21".
func jsNumberString(n float64) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		return "-" + jsNumberString(-n)
	}
	// The shortest digits that read back as n, as JS uses
	mantissa, exp, _ := strings.Cut(strconv.FormatFloat(n, 'e', -1, 64), "e")
	digits := strings.Replace(mantissa, ".", "", 1)
	e, _ := strconv.Atoi(exp)
	k := len(digits)
	// n = 0.<digits> * 10^point
	point := e + 1
	switch {
	case k <= point && point <= 21:
		return digits + strings.Repeat("0", point-k)
	case 0 < point && point <= 21:
		return digits[:point] + "." + digits[point:]
	case -6 < point && point <= 0:
		return "0." + strings.Repeat("0", -point) + digits
	}
	exponent := strconv.Itoa(point - 1)
	if point-1 >= 0 {
		exponent = "+" + exponent
	}
	if k == 1 {
		return digits + "e" + exponent
	}
	return digits[:1] + "." + digits[1:] + "e" + exponent
}
