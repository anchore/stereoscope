package internal

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestOrderableSet_Size(t *testing.T) {
	type testCase[T orderedComparable] struct {
		name string
		s    OrderableSet[T]
		want int
	}
	tests := []testCase[string]{
		{
			name: "empty set",
			s:    NewOrderableSet[string](),
			want: 0,
		},
		{
			name: "non-empty set",
			s:    NewOrderableSet[string]("items", "in", "set"),
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

func TestOrderableSet_Add(t *testing.T) {
	type args[T orderedComparable] struct {
		ids []T
	}
	type testCase[T orderedComparable] struct {
		name string
		s    OrderableSet[T]
		args args[T]
	}
	tests := []testCase[string]{
		{
			name: "add multiple",
			s:    NewOrderableSet[string](),
			args: args[string]{ids: []string{"a", "b", "c"}},
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

func TestOrderableSet_Remove(t *testing.T) {
	type args[T orderedComparable] struct {
		ids []T
	}
	type testCase[T orderedComparable] struct {
		name     string
		s        OrderableSet[T]
		args     args[T]
		expected []T
	}
	tests := []testCase[string]{
		{
			name:     "remove multiple",
			s:        NewOrderableSet[string]("a", "b", "c"),
			args:     args[string]{ids: []string{"a", "b"}},
			expected: []string{"c"},
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

func TestOrderableSet_Contains(t *testing.T) {
	type args[T orderedComparable] struct {
		i T
	}
	type testCase[T orderedComparable] struct {
		name string
		s    OrderableSet[T]
		args args[T]
		want bool
	}
	tests := []testCase[string]{
		{
			name: "contains",
			s:    NewOrderableSet[string]("a", "b", "c"),
			args: args[string]{i: "a"},
			want: true,
		},
		{
			name: "not contains",
			s:    NewOrderableSet[string]("a", "b", "c"),
			args: args[string]{i: "x"},
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

func TestOrderableSet_Clear(t *testing.T) {
	type testCase[T orderedComparable] struct {
		name string
		s    OrderableSet[T]
	}
	tests := []testCase[string]{
		{
			name: "go case",
			s:    NewOrderableSet[string]("a", "b", "c"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.s.Clear()
			assert.Equal(t, 0, tt.s.Size())
		})
	}
}

func TestOrderableSet_List(t *testing.T) {
	type testCase[T orderedComparable] struct {
		name string
		s    OrderableSet[T]
		want []T
	}
	tests := []testCase[string]{
		{
			name: "go case",
			s:    NewOrderableSet[string]("a", "b", "c"),
			want: []string{"a", "b", "c"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.ElementsMatchf(t, tt.want, tt.s.List(), "List()")
		})
	}
}

func TestOrderableSet_Sorted(t *testing.T) {
	type testCase[T orderedComparable] struct {
		name string
		s    OrderableSet[T]
		want []T
	}
	tests := []testCase[string]{
		{
			name: "go case",
			s:    NewOrderableSet[string]("a", "b", "c"),
			want: []string{"a", "b", "c"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, tt.s.Sorted(), "Sorted()")
		})
	}
}

func TestOrderableSet_ContainsAny(t *testing.T) {
	type args[T orderedComparable] struct {
		ids []T
	}
	type testCase[T orderedComparable] struct {
		name string
		s    OrderableSet[T]
		args args[T]
		want bool
	}
	tests := []testCase[string]{
		{
			name: "contains one",
			s:    NewOrderableSet[string]("a", "b", "c"),
			args: args[string]{ids: []string{"a", "x"}},
			want: true,
		},
		{
			name: "contains all",
			s:    NewOrderableSet[string]("a", "b", "c"),
			args: args[string]{ids: []string{"a", "b"}},
			want: true,
		},
		{
			name: "contains none",
			s:    NewOrderableSet[string]("a", "b", "c"),
			args: args[string]{ids: []string{"x", "y"}},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.s.ContainsAny(tt.args.ids...), fmt.Sprintf("ContainsAny(%v)", tt.args.ids))
		})
	}
}
