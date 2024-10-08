package growthbook

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestConditionValueIsPresent(t *testing.T) {
	client, _ := NewClient(context.TODO())
	condition := &Condition{}
	err := json.Unmarshal([]byte(`{"userId": "123"}`), condition)
	require.Nil(t, err)
	result := condition.eval(client, Attributes{"userId": "123"})
	require.True(t, result)
	result = condition.eval(client, Attributes{"userId": "42"})
	require.False(t, result)
}

func TestConditionValueNullOrNotPresent(t *testing.T) {
	client, _ := NewClient(context.TODO())
	condition := &Condition{}
	json.Unmarshal([]byte(`{"userId": null}`), condition)

	t.Run("attribute is nil", func(t *testing.T) {
		result := condition.eval(client, Attributes{"userId": nil})
		require.True(t, result)
	})

	t.Run("attribute not present", func(t *testing.T) {
		result := condition.eval(client, Attributes{})
		require.True(t, result)
	})

	t.Run("attribute is 0", func(t *testing.T) {
		result := condition.eval(client, Attributes{"userId": 0})
		require.False(t, result)
	})

	t.Run("attribute is empty string", func(t *testing.T) {
		result := condition.eval(client, Attributes{"userId": ""})
		require.False(t, result)
	})
}
