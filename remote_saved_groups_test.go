package growthbook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"testing"

	"github.com/growthbook/growthbook-golang/internal/condition"
	"github.com/stretchr/testify/require"
)

const remoteSavedGroupsJSON = `{
	"grp_vip": {"type": "remote", "attributeKey": "account_id"},
	"grp_vip:parent_id": {"type": "remote", "attributeKey": "parent_id", "valuesFrom": "grp_vip"},
	"grp_list": {"type": "list", "attributeKey": "id", "values": ["u_1"]}
}`

// GrowthBook guards "not in group" with $exists, as in the spec cases
const remoteFeaturesJSON = `{
	"in_vip": {"defaultValue": false, "rules": [{"condition": {"$savedGroup": {"id": "grp_vip"}}, "force": true}]},
	"not_in_vip": {"defaultValue": false, "rules": [{"condition": {
		"$not": {"$savedGroup": {"id": "grp_vip"}},
		"__gb_remoteGroupIds": {"$exists": true}
	}, "force": true}]}
}`

func remoteSavedGroups(t *testing.T, raw string) condition.SavedGroups {
	t.Helper()
	var groups condition.SavedGroups
	require.NoError(t, json.Unmarshal([]byte(raw), &groups))
	return groups
}

// remoteClient returns a client with the remote groups payload, and the buffer
// its logger writes to
func remoteClient(t *testing.T, opts ...ClientOption) (*Client, *bytes.Buffer) {
	t.Helper()
	var features FeatureMap
	require.NoError(t, json.Unmarshal([]byte(remoteFeaturesJSON), &features))
	logs := &bytes.Buffer{}
	base := []ClientOption{
		WithLogger(slog.New(slog.NewTextHandler(logs, nil))),
		WithFeatures(features),
		WithSavedGroups(remoteSavedGroups(t, remoteSavedGroupsJSON)),
		WithAttributes(Attributes{"account_id": "a1"}),
	}
	client, err := NewClient(context.Background(), append(base, opts...)...)
	require.NoError(t, err)
	return client, logs
}

func on(c *Client, key string) bool {
	return c.EvalFeature(context.Background(), key).On
}

func TestRemoteGroupIdsMatchTheGroupsTheyList(t *testing.T) {
	client, _ := remoteClient(t, WithRemoteGroupIds([]string{"grp_vip"}))
	require.Equal(t, []string{"grp_vip"}, client.RemoteGroupIds())
	require.True(t, on(client, "in_vip"))
	require.False(t, on(client, "not_in_vip"))
}

func TestRemoteGroupIdsUnresolvedFailEveryRuleUsingARemoteGroup(t *testing.T) {
	client, _ := remoteClient(t)
	require.Nil(t, client.RemoteGroupIds())
	require.False(t, on(client, "in_vip"))
	require.False(t, on(client, "not_in_vip"))
}

func TestRemoteGroupIdsEmptyIsInNoGroup(t *testing.T) {
	client, _ := remoteClient(t, WithRemoteGroupIds([]string{}))
	require.NotNil(t, client.RemoteGroupIds())
	require.False(t, on(client, "in_vip"))
	require.True(t, on(client, "not_in_vip"))
}

func TestRemoteGroupIdsWinOverTheAttributeWithoutChangingIt(t *testing.T) {
	attrs := Attributes{"account_id": "a1", "__gb_remoteGroupIds": []any{"grp_vip"}}
	client, _ := remoteClient(t, WithAttributes(attrs))
	require.True(t, on(client, "in_vip"))

	child, err := client.WithRemoteGroupIds([]string{})
	require.NoError(t, err)
	require.False(t, on(child, "in_vip"))
	// The parent and the caller's attributes are left as they were
	require.True(t, on(client, "in_vip"))
	require.Equal(t, Attributes{"account_id": "a1", "__gb_remoteGroupIds": []any{"grp_vip"}}, attrs)
}

func TestRemoteGroupIdsKeepApplyingToChildClients(t *testing.T) {
	client, _ := remoteClient(t, WithRemoteGroupIds([]string{"grp_vip"}))
	child, err := client.WithAttributes(Attributes{"account_id": "a2"})
	require.NoError(t, err)
	require.True(t, on(child, "in_vip"))
}

func TestRemoteGroupIdsAreCopied(t *testing.T) {
	ids := []string{"grp_vip"}
	client, _ := remoteClient(t, WithRemoteGroupIds(ids))
	ids[0] = "grp_other"
	require.True(t, on(client, "in_vip"))
}

func TestInGroupOperatorsOnARemoteGroupAreUndecided(t *testing.T) {
	groups := remoteSavedGroups(t, remoteSavedGroupsJSON)
	attrs := Attributes{"account_id": "a1", "__gb_remoteGroupIds": []any{"grp_vip"}}
	for _, raw := range []string{`{"account_id": {"$inGroup": "grp_vip"}}`, `{"account_id": {"$notInGroup": "grp_vip"}}`} {
		var cond condition.Base
		require.NoError(t, json.Unmarshal([]byte(raw), &cond))
		require.False(t, cond.Eval(attributesValue(attrs), groups), raw)
	}
}

func TestRemoteGroupIdsMissingWarningIsLoggedOnceWithoutAttributeValues(t *testing.T) {
	client, logs := remoteClient(t, WithAttributes(Attributes{"account_id": "secret-account"}))
	on(client, "in_vip")
	child, err := client.WithAttributes(Attributes{"account_id": "secret-account-2"})
	require.NoError(t, err)
	on(child, "in_vip")
	child.RunExperiment(context.Background(), &Experiment{Key: "e", Variations: []FeatureValue{0, 1}})

	require.Equal(t, 1, bytes.Count(logs.Bytes(), []byte("no remote group ids")))
	require.NotContains(t, logs.String(), "secret-account")
}

func TestRemoteGroupIdsMissingWarningIsNotLoggedWhenTheIdsAreKnown(t *testing.T) {
	client, logs := remoteClient(t, WithRemoteGroupIds([]string{}))
	on(client, "in_vip")
	require.NotContains(t, logs.String(), "no remote group ids")

	client, logs = remoteClient(t, WithAttributes(Attributes{"__gb_remoteGroupIds": []any{}}))
	on(client, "in_vip")
	require.NotContains(t, logs.String(), "no remote group ids")

	client, logs = remoteClient(t, WithSavedGroups(remoteSavedGroups(t, `{"grp_list": {"type": "list", "attributeKey": "id", "values": ["u_1"]}}`)))
	on(client, "in_vip")
	require.NotContains(t, logs.String(), "no remote group ids")
}

func TestAddRemoteSavedGroupsPassesOnlyTheAttributesRemoteGroupsUse(t *testing.T) {
	client, _ := remoteClient(t, WithAttributes(Attributes{"account_id": "a1", "email": "user@example.test", "id": "u_1"}))
	var gotAttrs Attributes
	var gotGroups []RemoteSavedGroup
	child, err := client.AddRemoteSavedGroups(context.Background(), func(_ context.Context, attrs Attributes, groups []RemoteSavedGroup) ([]string, error) {
		gotAttrs, gotGroups = attrs, groups
		return []string{"grp_vip"}, nil
	})
	require.NoError(t, err)
	require.Equal(t, Attributes{"account_id": "a1"}, gotAttrs)
	require.Equal(t, []RemoteSavedGroup{
		{ID: "grp_vip", AttributeKey: "account_id"},
		{ID: "grp_vip:parent_id", AttributeKey: "parent_id", ValuesFrom: "grp_vip"},
	}, gotGroups)
	require.Equal(t, []string{"grp_vip"}, child.RemoteGroupIds())
	require.True(t, on(child, "in_vip"))
	// A child: the client itself is unchanged
	require.Nil(t, client.RemoteGroupIds())
}

func TestAddRemoteSavedGroupsReadsADotSeparatedAttributeKeyAsAPath(t *testing.T) {
	client, _ := remoteClient(t,
		WithAttributes(Attributes{"company": map[string]any{"id": "c1"}}),
		WithSavedGroups(remoteSavedGroups(t, `{"grp_co": {"type": "remote", "attributeKey": "company.id"}}`)),
	)
	var got Attributes
	_, err := client.AddRemoteSavedGroups(context.Background(), func(_ context.Context, attrs Attributes, _ []RemoteSavedGroup) ([]string, error) {
		got = attrs
		return nil, nil
	})
	require.NoError(t, err)
	require.Equal(t, Attributes{"company.id": "c1"}, got)
}

func TestAddRemoteSavedGroupsSetsAnEmptyListForANilResult(t *testing.T) {
	client, _ := remoteClient(t)
	child, err := client.AddRemoteSavedGroups(context.Background(), func(context.Context, Attributes, []RemoteSavedGroup) ([]string, error) {
		return nil, nil
	})
	require.NoError(t, err)
	require.Equal(t, []string{}, child.RemoteGroupIds())
}

func TestAddRemoteSavedGroupsSkipsTheResolverWithoutRemoteGroups(t *testing.T) {
	client, _ := remoteClient(t, WithSavedGroups(remoteSavedGroups(t, `{"grp_list": {"type": "list", "attributeKey": "id", "values": ["u_1"]}}`)))
	child, err := client.AddRemoteSavedGroups(context.Background(), func(context.Context, Attributes, []RemoteSavedGroup) ([]string, error) {
		t.Fatal("not called")
		return nil, nil
	})
	require.NoError(t, err)
	require.Same(t, client, child)
	require.Nil(t, child.RemoteGroupIds())
}

func TestAddRemoteSavedGroupsReturnsTheClientUnchangedWhenTheResolverFails(t *testing.T) {
	client, _ := remoteClient(t, WithRemoteGroupIds([]string{"grp_old"}))
	failure := errors.New("redis down")
	child, err := client.AddRemoteSavedGroups(context.Background(), func(context.Context, Attributes, []RemoteSavedGroup) ([]string, error) {
		return nil, failure
	})
	require.ErrorIs(t, err, failure)
	require.Same(t, client, child)
	require.Equal(t, []string{"grp_old"}, child.RemoteGroupIds())
}

func TestAddRemoteSavedGroupsLooksUpAgainWhenTheSavedGroupsChange(t *testing.T) {
	client, _ := remoteClient(t)
	var seen [][]string
	child, err := client.AddRemoteSavedGroups(context.Background(), func(_ context.Context, _ Attributes, groups []RemoteSavedGroup) ([]string, error) {
		ids := make([]string, len(groups))
		for i, g := range groups {
			ids[i] = g.ID
		}
		seen = append(seen, ids)
		if len(seen) == 1 {
			require.NoError(t, client.UpdateFromApiResponseJSON(`{"savedGroups": {
				"grp_vip": {"type": "remote", "attributeKey": "account_id"},
				"grp_new": {"type": "remote", "attributeKey": "account_id"}
			}}`))
			return []string{"grp_vip"}, nil
		}
		return []string{"grp_vip", "grp_new"}, nil
	})
	require.NoError(t, err)
	require.Equal(t, [][]string{{"grp_vip", "grp_vip:parent_id"}, {"grp_new", "grp_vip"}}, seen)
	require.Equal(t, []string{"grp_vip", "grp_new"}, child.RemoteGroupIds())
}

func TestAddRemoteSavedGroupsLeavesTheIdsWhenTheNewSavedGroupsHaveNoRemoteGroups(t *testing.T) {
	client, _ := remoteClient(t)
	child, err := client.AddRemoteSavedGroups(context.Background(), func(context.Context, Attributes, []RemoteSavedGroup) ([]string, error) {
		require.NoError(t, client.UpdateFromApiResponseJSON(`{"savedGroups": {}}`))
		return []string{"grp_vip"}, nil
	})
	require.NoError(t, err)
	require.Nil(t, child.RemoteGroupIds())
}

func TestPruneLeavesARemoteGroupUndecidedWhileTheIdsAreUnknown(t *testing.T) {
	var groups map[string]any
	require.NoError(t, json.Unmarshal([]byte(remoteSavedGroupsJSON), &groups))
	cond := map[string]any{"$savedGroup": map[string]any{"id": "grp_vip"}}

	res, err := PruneCondition(cond, Attributes{"account_id": "a1"}, groups)
	require.NoError(t, err)
	require.Equal(t, cond, res)
	res, err = PruneCondition(cond, Attributes{"__gb_remoteGroupIds": []any{"grp_vip"}}, groups)
	require.NoError(t, err)
	require.Equal(t, true, res)
	res, err = PruneCondition(cond, Attributes{"__gb_remoteGroupIds": []any{}}, groups)
	require.NoError(t, err)
	require.Equal(t, false, res)
	res, err = PruneCondition(map[string]any{"$savedGroup": map[string]any{"id": "grp_vip", "attributeKey": "parent_id"}}, Attributes{}, groups)
	require.NoError(t, err)
	require.Equal(t, false, res)
}

func TestClientReducePayloadUsesTheRemoteGroupIds(t *testing.T) {
	client, _ := remoteClient(t)
	payload := `{"features": ` + remoteFeaturesJSON + `, "savedGroups": ` + remoteSavedGroupsJSON + `}`
	require.NoError(t, client.UpdateFromApiResponseJSON(payload))

	reduce := func(c *Client) map[string]any {
		out, err := c.ReducePayload(ReducePayloadOptions{})
		require.NoError(t, err)
		var reduced map[string]any
		require.NoError(t, json.Unmarshal(out, &reduced))
		return reduced
	}
	// Unknown ids keep the rule and the group it uses
	reduced := reduce(client)
	require.Contains(t, reduced["savedGroups"], "grp_vip")
	require.Len(t, reduced["features"].(map[string]any)["in_vip"].(map[string]any)["rules"], 1)

	// Known ids decide it: "in group" can't match, so its rule never matches,
	// and "not in group" always does
	child, err := client.WithRemoteGroupIds([]string{})
	require.NoError(t, err)
	reduced = reduce(child)
	features := reduced["features"].(map[string]any)
	require.Equal(t, map[string]any{"$not": map[string]any{}}, features["in_vip"].(map[string]any)["rules"].([]any)[0].(map[string]any)["condition"])
	require.NotContains(t, features["not_in_vip"].(map[string]any)["rules"].([]any)[0], "condition")
	require.Empty(t, reduced["savedGroups"])
}

// fakeRedis records each SMEMBERS key
type fakeRedis struct {
	sets map[string][]string
	keys []string
	err  error
}

func (r *fakeRedis) SMembers(_ context.Context, key string) ([]string, error) {
	r.keys = append(r.keys, key)
	return r.sets[key], r.err
}

var redisTestGroups = []RemoteSavedGroup{
	{ID: "grp_vip", AttributeKey: "account_id"},
	{ID: "grp_vip:parent_id", AttributeKey: "parent_id", ValuesFrom: "grp_vip"},
}

func resolve(t *testing.T, redis *fakeRedis, attrs Attributes, groups []RemoteSavedGroup) []string {
	t.Helper()
	ids, err := RedisResolver(redis)(context.Background(), attrs, groups)
	require.NoError(t, err)
	return ids
}

func TestRedisResolverLooksUpEachAttributeValueInItsGroup(t *testing.T) {
	redis := &fakeRedis{sets: map[string][]string{"gb:member:account_id:a1": {"grp_vip", "grp_other"}}}
	require.Equal(t, []string{"grp_vip"}, resolve(t, redis, Attributes{"account_id": "a1"}, redisTestGroups))
	require.Equal(t, []string{"gb:member:account_id:a1"}, redis.keys)
}

func TestRedisResolverChecksADerivedGroupInItsSourceGroupsSets(t *testing.T) {
	redis := &fakeRedis{sets: map[string][]string{"gb:member:account_id:p1": {"grp_vip"}}}
	ids := resolve(t, redis, Attributes{"account_id": "a1", "parent_id": "p1"}, redisTestGroups)
	require.Equal(t, []string{"grp_vip:parent_id"}, ids)
}

func TestRedisResolverSkipsADerivedGroupWhoseSourceIsMissing(t *testing.T) {
	redis := &fakeRedis{sets: map[string][]string{"gb:member:parent_id:p1": {"grp_vip:parent_id"}}}
	require.Empty(t, resolve(t, redis, Attributes{"parent_id": "p1"}, redisTestGroups[1:]))
	require.Empty(t, redis.keys)
}

func TestRedisResolverKeepsKeysCollidingOnAColonApart(t *testing.T) {
	groups := []RemoteSavedGroup{{ID: "grp_a", AttributeKey: "a:b"}, {ID: "grp_ab", AttributeKey: "a"}}
	// `a:b` + `c` and `a` + `b:c` both read gb:member:a:b:c
	redis := &fakeRedis{sets: map[string][]string{"gb:member:a:b:c": {"grp_a"}}}
	require.Equal(t, []string{"grp_a"}, resolve(t, redis, Attributes{"a:b": "c", "a": "b:c"}, groups))
	require.Equal(t, []string{"gb:member:a:b:c"}, redis.keys)
}

func TestRedisResolverLooksUpEachItemOfASliceOncePerKey(t *testing.T) {
	redis := &fakeRedis{sets: map[string][]string{"gb:member:account_id:a2": {"grp_vip"}}}
	require.Equal(t, []string{"grp_vip"}, resolve(t, redis, Attributes{"account_id": []any{"a1", "a2", "a1"}}, redisTestGroups[:1]))
	require.Equal(t, []string{"gb:member:account_id:a1", "gb:member:account_id:a2"}, redis.keys)
}

func TestRedisResolverFormatsNumbersLikeJSAndSkipsOtherValues(t *testing.T) {
	redis := &fakeRedis{}
	values := []any{1, 2.0, 1.5, 0.00001, 1e21, math.Copysign(0, -1), math.NaN(), math.Inf(1), true, nil, map[string]any{"a": 1}}
	resolve(t, redis, Attributes{"account_id": values}, redisTestGroups[:1])
	require.Equal(t, []string{
		"gb:member:account_id:1",
		"gb:member:account_id:2",
		"gb:member:account_id:1.5",
		"gb:member:account_id:0.00001",
		"gb:member:account_id:1e+21",
		"gb:member:account_id:0",
	}, redis.keys)
}

func TestJSNumberString(t *testing.T) {
	for n, s := range map[float64]string{
		0.1: "0.1", 1e-7: "1e-7", 1.5e-7: "1.5e-7", 0.000001: "0.000001", 1e20: "100000000000000000000",
		123456789012345680000: "123456789012345680000", 1.7976931348623157e308: "1.7976931348623157e+308",
		5e-324: "5e-324", -2.5: "-2.5", 12345.678: "12345.678",
	} {
		require.Equal(t, s, jsNumberString(n), "%v", n)
	}
	// At run time: a constant 0.1 + 0.2 is exactly 0.3
	a, b := 0.1, 0.2
	require.Equal(t, "0.30000000000000004", jsNumberString(a+b))
}

func TestRedisResolverReturnsTheRedisError(t *testing.T) {
	failure := errors.New("connection refused")
	_, err := RedisResolver(&fakeRedis{err: failure})(context.Background(), Attributes{"account_id": "a1"}, redisTestGroups[:1])
	require.ErrorIs(t, err, failure)
}

func TestRedisResolverWorksThroughAddRemoteSavedGroups(t *testing.T) {
	client, _ := remoteClient(t)
	redis := &fakeRedis{sets: map[string][]string{"gb:member:account_id:a1": {"grp_vip"}}}
	child, err := client.AddRemoteSavedGroups(context.Background(), RedisResolver(redis))
	require.NoError(t, err)
	require.Equal(t, []string{"grp_vip"}, child.RemoteGroupIds())
	require.True(t, on(child, "in_vip"))
}
