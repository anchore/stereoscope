package file

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestPathSet_Size(t *testing.T) {
	type testCase struct {
		name string
		s    PathSet
		want int
	}
	tests := []testCase{
		{
			name: "empty set",
			s:    NewPathSet(),
			want: 0,
		},
		{
			name: "non-empty set",
			s:    NewPathSet("items", "in", "set"),
			want: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.Size(); got != tt.want {
				t.Errorf("Size() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPathSet_Add(t *testing.T) {
	type args struct {
		ids []Path
	}
	type testCase struct {
		name string
		s    PathSet
		args args
	}
	tests := []testCase{
		{
			name: "add multiple",
			s:    NewPathSet(),
			args: args{ids: []Path{"a", "b", "c"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.s.Add(tt.args.ids...)
			for _, id := range tt.args.ids {
				if !tt.s.Contains(id) {
					t.Errorf("expected set to contain %q", id)
				}
			}
		})
	}
}

func TestPathSet_Remove(t *testing.T) {
	type args struct {
		ids []Path
	}
	type testCase struct {
		name     string
		s        PathSet
		args     args
		expected []Path
	}
	tests := []testCase{
		{
			name:     "remove multiple",
			s:        NewPathSet("a", "b", "c"),
			args:     args{ids: []Path{"a", "b"}},
			expected: []Path{"c"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.s.Remove(tt.args.ids...)
			for _, id := range tt.args.ids {
				if tt.s.Contains(id) {
					t.Errorf("expected set to NOT contain %q", id)
				}
			}
			for _, id := range tt.expected {
				if !tt.s.Contains(id) {
					t.Errorf("expected set to contain %q", id)
				}
			}
		})
	}
}

func TestPathSet_Contains(t *testing.T) {
	type args struct {
		i Path
	}
	type testCase struct {
		name string
		s    PathSet
		args args
		want bool
	}
	tests := []testCase{
		{
			name: "contains",
			s:    NewPathSet("a", "b", "c"),
			args: args{i: "a"},
			want: true,
		},
		{
			name: "not contains",
			s:    NewPathSet("a", "b", "c"),
			args: args{i: "x"},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.Contains(tt.args.i); got != tt.want {
				t.Errorf("Contains() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPathSet_Clear(t *testing.T) {
	type testCase struct {
		name string
		s    PathSet
	}
	tests := []testCase{
		{
			name: "go case",
			s:    NewPathSet("a", "b", "c"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.s.Clear()
			assert.Equal(t, 0, tt.s.Size())
		})
	}
}

func TestPathSet_List(t *testing.T) {
	type testCase struct {
		name string
		s    PathSet
		want []Path
	}
	tests := []testCase{
		{
			name: "go case",
			s:    NewPathSet("a", "b", "c"),
			want: []Path{"a", "b", "c"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.ElementsMatchf(t, tt.want, tt.s.List(), "List()")
		})
	}
}

func TestPathSet_Sorted(t *testing.T) {
	type testCase struct {
		name string
		s    PathSet
		want []Path
	}
	tests := []testCase{
		{
			name: "go case",
			s:    NewPathSet("a", "b", "c"),
			want: []Path{"a", "b", "c"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, tt.s.Sorted(), "Sorted()")
		})
	}
}

func TestPathSet_ContainsAny(t *testing.T) {
	type args struct {
		ids []Path
	}
	type testCase struct {
		name string
		s    PathSet
		args args
		want bool
	}
	tests := []testCase{
		{
			name: "contains one",
			s:    NewPathSet("a", "b", "c"),
			args: args{ids: []Path{"a", "x"}},
			want: true,
		},
		{
			name: "contains all",
			s:    NewPathSet("a", "b", "c"),
			args: args{ids: []Path{"a", "b"}},
			want: true,
		},
		{
			name: "contains none",
			s:    NewPathSet("a", "b", "c"),
			args: args{ids: []Path{"x", "y"}},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.s.ContainsAny(tt.args.ids...), fmt.Sprintf("ContainsAny(%v)", tt.args.ids))
		})
	}
}

func TestPathCountSet(t *testing.T) {
	s := NewPathCountSet()

	s.Add("a", "b") // {a: 1, b: 1}
	assert.True(t, s.Contains("a"))
	assert.True(t, s.Contains("b"))
	assert.False(t, s.Contains("c"))

	s.Add("a", "c") // {a: 2, b: 1, c: 1}
	assert.True(t, s.Contains("a"))
	assert.True(t, s.Contains("b"))
	assert.True(t, s.Contains("c"))

	s.Remove("a") // {a: 1, b: 1, c: 1}
	assert.True(t, s.Contains("a"))
	assert.True(t, s.Contains("b"))
	assert.True(t, s.Contains("c"))

	s.Remove("a", "b") // {c: 1}
	assert.False(t, s.Contains("a"))
	assert.False(t, s.Contains("b"))
	assert.True(t, s.Contains("c"))

	s.Remove("a", "b", "v", "c") // {}
	assert.False(t, s.Contains("a"))
	assert.False(t, s.Contains("b"))
	assert.False(t, s.Contains("c"))

	s.Add("a", "a", "a", "a") // {a: 4}
	assert.True(t, s.Contains("a"))
	assert.Equal(t, 4, s["a"])

	s.Remove("a", "a", "a") // {a: 1}
	assert.True(t, s.Contains("a"))

	s.Remove("a", "a", "a", "a") // {}
	assert.False(t, s.Contains("a"))

	s.Remove("a", "a", "a", "a", "a", "a", "a", "a") // {}
	assert.False(t, s.Contains("a"))
}
