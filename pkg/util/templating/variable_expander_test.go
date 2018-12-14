package templating

import (
	"errors"
	"fmt"
	"testing"

	"github.com/atlassian/voyager/pkg/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func simpleResolver(s string) (interface{}, error) {
	return fmt.Sprintf("res-%s", s), nil
}

func simpleNumberResolver(r interface{}) VariableResolver {
	return func(s string) (interface{}, error) {
		return r, nil
	}
}

func mappedResolver(vars map[string]interface{}) VariableResolver {
	return func(s string) (interface{}, error) {
		return vars[s], nil
	}
}

var expandSimpleSuccessTests = []struct {
	in  string
	out string
}{
	{"no-variable", "no-variable"},
	{"${a}", "res-a"},
	{"with-${variable}", "with-res-variable"},
	{"multiple-${first}-${second}", "multiple-res-first-res-second"},
	{"nested-${first-${only}}", "nested-res-first-res-only"},
	{"${adj1}${adj2}", "res-adj1res-adj2"},
	{"missing-start}", "missing-start}"},
	{"$${escaped}", "${escaped}"},
	{"midline $${escaped}", "midline ${escaped}"},
	{`some ${multi}
	${line} values`, `some res-multi
	res-line values`},
}

var expandSimpleFailureTests = []struct {
	in         string
	errorCount int
}{
	{"${missing-end", 1},
	{"invalid ${var-{contents}}", 1},
	{"num only ${1}", 1},
	{"num start ${1start}", 1},
	{"_ only ${_}", 1},
	{"_ start ${_start}", 1},
	{"- only ${-}", 1},
	{"- start ${-start}", 1},
	{"- start ${-invalid} and ${3}", 2},
}

func TestSimpleVariableExpansionsSuccess(t *testing.T) {
	t.Parallel()
	expander := NewVariableExpander(simpleResolver, "")

	for _, tc := range expandSimpleSuccessTests {
		t.Run(tc.in, func(t *testing.T) {
			result, err := expander.Expand(tc.in)
			require.Nil(t, err)
			assert.Equal(t, tc.out, result)
		})
	}
}

func TestSimpleVariableExpansionsFailure(t *testing.T) {
	t.Parallel()
	expander := NewVariableExpander(simpleResolver, "")

	for _, tc := range expandSimpleFailureTests {
		t.Run(tc.in, func(t *testing.T) {
			_, err := expander.Expand(tc.in)
			require.NotNil(t, err)
			require.Len(t, err.ErrorList, tc.errorCount)
		})
	}
}

func TestSimpleResolveError(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("some error")

	errResolver := func(s string) (interface{}, error) {
		return "", expectedErr
	}

	expander := NewVariableExpander(errResolver, "")

	_, errs := expander.Expand("just ${need} variable")

	assert.Len(t, errs.ErrorList, 1)
	assert.EqualError(t, errs.ErrorList[0], expectedErr.Error())
}

func TestMultipleResolveErrors(t *testing.T) {
	t.Parallel()

	firstErr := util.NewErrInvalidVariableName("need")
	secondErr := util.NewErrVariableNotFound("two", "")

	callCount := 0
	errResolver := func(s string) (interface{}, error) {
		callCount++
		if callCount == 1 {
			return "", firstErr
		}

		return "", secondErr
	}

	expander := NewVariableExpander(errResolver, "")

	_, errs := expander.Expand("just ${need} ${two}")

	require.Len(t, errs.ErrorList, 2)
	assert.Contains(t, errs.ErrorList, firstErr)
	assert.Contains(t, errs.ErrorList, secondErr)
}

func TestNumberToString(t *testing.T) {
	t.Parallel()

	prefixes := []string{"", "something:"}

	for _, prefix := range prefixes {
		expander := NewVariableExpander(simpleNumberResolver(15), prefix)
		result, errs := expander.Expand(fmt.Sprintf("just ${%sneed} ${%stwo}", prefix, prefix))
		require.Nil(t, errs)
		assert.Equal(t, "just 15 15", result)
	}
}

func TestVariableIsNumber(t *testing.T) {
	t.Parallel()

	prefixes := []string{"", "something:"}

	for _, prefix := range prefixes {
		expander := NewVariableExpander(simpleNumberResolver(15), prefix)
		result, errs := expander.Expand(fmt.Sprintf("${%saNumber}", prefix))
		require.Nil(t, errs)
		assert.Equal(t, 15, result)
	}
}

func TestStringCanConcatStrings(t *testing.T) {
	t.Parallel()

	vars := map[string]interface{}{
		"one": "one",
		"two": "two",
	}
	prefixes := []string{"", "something:"}

	for _, prefix := range prefixes {
		expander := NewVariableExpander(mappedResolver(vars), prefix)
		result, errs := expander.Expand(fmt.Sprintf("${%sone}${%stwo}", prefix, prefix))
		require.Nil(t, errs)
		assert.Equal(t, "onetwo", result)
	}
}

func TestStringCanConcatStringAndNumber(t *testing.T) {
	t.Parallel()

	vars := map[string]interface{}{
		"one": "one",
		"two": 2,
	}
	prefixes := []string{"", "something:"}

	for _, prefix := range prefixes {
		expander := NewVariableExpander(mappedResolver(vars), prefix)
		result, errs := expander.Expand(fmt.Sprintf("${%sone}${%stwo}", prefix, prefix))
		require.Nil(t, errs)
		assert.Equal(t, "one2", result)
	}
}

func TestStringCanConcatNumberAndString(t *testing.T) {
	t.Parallel()

	vars := map[string]interface{}{
		"one": 1,
		"two": "two",
	}
	prefixes := []string{"", "something:"}

	for _, prefix := range prefixes {
		expander := NewVariableExpander(mappedResolver(vars), prefix)
		result, errs := expander.Expand(fmt.Sprintf("${%sone}${%stwo}", prefix, prefix))
		require.Nil(t, errs)
		assert.Equal(t, "1two", result)
	}
}

func TestCheckPrefix(t *testing.T) {
	t.Parallel()

	vars := map[string]interface{}{
		"one": 1,
		"two": "two",
	}
	expander := NewVariableExpander(mappedResolver(vars), "prefix.")

	result := expander.ValidPrefix("${prefix.two}")
	assert.True(t, result)

	result = expander.ValidPrefix("${prefix.one}_static_text_${prefix.two}")
	assert.True(t, result)

	result = expander.ValidPrefix("${notgood.two}")
	assert.False(t, result)
}
