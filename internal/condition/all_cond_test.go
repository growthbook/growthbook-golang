package condition

import (
	"testing"

	"github.com/growthbook/growthbook-golang/internal/value"
	"github.com/stretchr/testify/require"
)

func TestAllConds(t *testing.T) {
	eq1 := NewCompCond(eqOp, 1)
	eq2 := NewCompCond(eqOp, 2)
	gt10 := NewCompCond(gtOp, 10)

	cond := AllConds{eq1, eq2, gt10}
	require.True(t, cond.Eval(value.Arr(2, 20, 1, 5), nil, nil))
	require.False(t, cond.Eval(value.Arr(1, 5, 1, 50), nil, nil))
}

func TestAlliConds(t *testing.T) {
	t.Run("case-insensitive string matching", func(t *testing.T) {
		apple := NewValueCondCaseInsensitive("apple")
		banana := NewValueCondCaseInsensitive("banana")
		cond := AlliConds{apple, banana}

		require.True(t, cond.Eval(value.Arr("APPLE", "BANANA", "cherry"), nil, nil))
		require.True(t, cond.Eval(value.Arr("Apple", "Banana"), nil, nil))
		require.False(t, cond.Eval(value.Arr("APPLE", "cherry"), nil, nil))
		require.False(t, cond.Eval(value.Arr("grape", "orange"), nil, nil))
	})

	t.Run("mixed string and numeric values", func(t *testing.T) {
		apple := NewValueCondCaseInsensitive("apple")
		num1 := NewValueCondCaseInsensitive(1)
		cond := AlliConds{apple, num1}

		require.True(t, cond.Eval(value.Arr("APPLE", 1, 2), nil, nil))
		require.False(t, cond.Eval(value.Arr("APPLE", 2), nil, nil))
	})

	t.Run("non-array returns false", func(t *testing.T) {
		apple := NewValueCondCaseInsensitive("apple")
		cond := AlliConds{apple}

		require.False(t, cond.Eval(value.New("apple"), nil, nil))
		require.False(t, cond.Eval(value.New(123), nil, nil))
	})

	t.Run("empty conditions returns true", func(t *testing.T) {
		cond := AlliConds{}
		require.True(t, cond.Eval(value.Arr("test"), nil, nil))
	})
}
