package internal

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStringSet_Size(t *testing.T) {
	type testCase struct {
		name string
		s    StringSet
		want int
	}
	tests := []testCase{
		{
			name: "empty set",
			s:    NewStringSet(),
			want: 0,
		},
		{
			name: "non-empty set",
			s:    NewStringSet("items", "in", "set"),
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

func TestStringSet_Add(t *testing.T) {
	type args struct {
		ids []string
	}
	type testCase struct {
		name string
		s    StringSet
		args args
	}
	tests := []testCase{
		{
			name: "add multiple",
			s:    NewStringSet(),
			args: args{ids: []string{"a", "b", "c"}},
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

func TestStringSet_Remove(t *testing.T) {
	type args struct {
		ids []string
	}
	type testCase struct {
		name     string
		s        StringSet
		args     args
		expected []string
	}
	tests := []testCase{
		{
			name:     "remove multiple",
			s:        NewStringSet("a", "b", "c"),
			args:     args{ids: []string{"a", "b"}},
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

func TestStringSet_Contains(t *testing.T) {
	type args struct {
		i string
	}
	type testCase struct {
		name string
		s    StringSet
		args args
		want bool
	}
	tests := []testCase{
		{
			name: "contains",
			s:    NewStringSet("a", "b", "c"),
			args: args{i: "a"},
			want: true,
		},
		{
			name: "not contains",
			s:    NewStringSet("a", "b", "c"),
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

func TestStringSet_Clear(t *testing.T) {
	type testCase struct {
		name string
		s    StringSet
	}
	tests := []testCase{
		{
			name: "go case",
			s:    NewStringSet("a", "b", "c"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.s.Clear()
			assert.Equal(t, 0, tt.s.Size())
		})
	}
}

func TestStringSet_List(t *testing.T) {
	type testCase struct {
		name string
		s    StringSet
		want []string
	}
	tests := []testCase{
		{
			name: "go case",
			s:    NewStringSet("a", "b", "c"),
			want: []string{"a", "b", "c"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.ElementsMatchf(t, tt.want, tt.s.List(), "List()")
		})
	}
}

func TestStringSet_Sorted(t *testing.T) {
	type testCase struct {
		name string
		s    StringSet
		want []string
	}
	tests := []testCase{
		{
			name: "go case",
			s:    NewStringSet("a", "b", "c"),
			want: []string{"a", "b", "c"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, tt.s.Sorted(), "Sorted()")
		})
	}
}

func TestStringSet_ContainsAny(t *testing.T) {
	type args struct {
		ids []string
	}
	type testCase struct {
		name string
		s    StringSet
		args args
		want bool
	}
	tests := []testCase{
		{
			name: "contains one",
			s:    NewStringSet("a", "b", "c"),
			args: args{ids: []string{"a", "x"}},
			want: true,
		},
		{
			name: "contains all",
			s:    NewStringSet("a", "b", "c"),
			args: args{ids: []string{"a", "b"}},
			want: true,
		},
		{
			name: "contains none",
			s:    NewStringSet("a", "b", "c"),
			args: args{ids: []string{"x", "y"}},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.s.ContainsAny(tt.args.ids...), fmt.Sprintf("ContainsAny(%v)", tt.args.ids))
		})
	}
}
