package tagged

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Tagged(t *testing.T) {
	set := Values[int]{
		New(1, "one"),
		New(2, "two", "second"),
		New(3, "three", "third"),
		New(23, "twenty-three", "twenty", "third"),
		New(4, "four", ""),
		New(9, "nine"),
	}

	tests := []struct {
		name     string
		keep     []string
		remove   []string
		expected []int
	}{
		{
			name:     "by tag",
			keep:     arr("two"),
			expected: arr(2),
		},
		{
			name:     "by multiple",
			keep:     arr("one", "third"),
			expected: arr(1, 3, 23),
		},
		{
			name:     "nil keep",
			keep:     nil,
			expected: nil, // arr(1, 2, 3, 23, 4, 9),
		},
		{
			name:     "empty keep",
			keep:     []string{},
			expected: nil, // arr(1, 2, 3, 23, 4, 9),
		},
		{
			name:     "remove by tag",
			keep:     arr("one", "twenty-three"),
			remove:   arr("third"),
			expected: arr(1),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := set.Select(test.keep...).Remove(test.remove...)
			if test.expected == nil {
				require.Empty(t, got)
				return
			}
			require.ElementsMatch(t, test.expected, got.Collect())
		})
	}
}

func arr[T any](v ...T) []T {
	return v
}
