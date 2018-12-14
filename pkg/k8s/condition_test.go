package k8s

import (
	"testing"

	cond_v1 "github.com/atlassian/ctrl/apis/condition/v1"
	"github.com/stretchr/testify/require"
)

func TestCalculateConditionAny(t *testing.T) {
	t.Parallel()

	t.Run("returns false on empty conditions", func(t *testing.T) {
		require.Equal(t, cond_v1.ConditionFalse, CalculateConditionAny([]cond_v1.Condition{}))
	})

	t.Run("returns true if any trues", func(t *testing.T) {
		conditions := []cond_v1.Condition{
			{
				Status: cond_v1.ConditionFalse,
			},
			{
				Status: cond_v1.ConditionFalse,
			},
			{
				Status: cond_v1.ConditionTrue,
			},
		}
		require.Equal(t, cond_v1.ConditionTrue, CalculateConditionAny(conditions))
	})

	t.Run("returns unknown if no trues and has unknown", func(t *testing.T) {
		conditions := []cond_v1.Condition{
			{
				Status: cond_v1.ConditionFalse,
			},
			{
				Status: cond_v1.ConditionUnknown,
			},
			{
				Status: cond_v1.ConditionFalse,
			},
		}
		require.Equal(t, cond_v1.ConditionUnknown, CalculateConditionAny(conditions))
	})

	t.Run("returns false if only falses", func(t *testing.T) {
		conditions := []cond_v1.Condition{
			{
				Status: cond_v1.ConditionFalse,
			},
			{
				Status: cond_v1.ConditionFalse,
			},
			{
				Status: cond_v1.ConditionFalse,
			},
		}
		require.Equal(t, cond_v1.ConditionFalse, CalculateConditionAny(conditions))
	})
}

func TestCalculateConditionAll(t *testing.T) {
	t.Parallel()

	t.Run("returns unknown on empty conditions", func(t *testing.T) {
		require.Equal(t, cond_v1.ConditionUnknown, CalculateConditionAll([]cond_v1.Condition{}))
	})

	t.Run("returns false if any false", func(t *testing.T) {
		conditions := []cond_v1.Condition{
			{
				Status: cond_v1.ConditionFalse,
			},
			{
				Status: cond_v1.ConditionTrue,
			},
			{
				Status: cond_v1.ConditionTrue,
			},
		}
		require.Equal(t, cond_v1.ConditionFalse, CalculateConditionAll(conditions))
	})

	t.Run("returns unknown if any unknown", func(t *testing.T) {
		conditions := []cond_v1.Condition{
			{
				Status: cond_v1.ConditionFalse,
			},
			{
				Status: cond_v1.ConditionTrue,
			},
			{
				Status: cond_v1.ConditionUnknown,
			},
		}
		require.Equal(t, cond_v1.ConditionUnknown, CalculateConditionAll(conditions))
	})

	t.Run("returns false if no unknowns and has false", func(t *testing.T) {
		conditions := []cond_v1.Condition{
			{
				Status: cond_v1.ConditionFalse,
			},
			{
				Status: cond_v1.ConditionTrue,
			},
			{
				Status: cond_v1.ConditionTrue,
			},
		}
		require.Equal(t, cond_v1.ConditionFalse, CalculateConditionAll(conditions))
	})

	t.Run("returns true if only trues", func(t *testing.T) {
		conditions := []cond_v1.Condition{
			{
				Status: cond_v1.ConditionTrue,
			},
			{
				Status: cond_v1.ConditionTrue,
			},
			{
				Status: cond_v1.ConditionTrue,
			},
		}
		require.Equal(t, cond_v1.ConditionTrue, CalculateConditionAll(conditions))
	})

}

func TestString(t *testing.T) {
	t.Parallel()

	condition := cond_v1.Condition{}
	require.NotEmpty(t, condition.String())
}
