package node

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIDSet_Size(t *testing.T) {
	type testCase struct {
		name string
		s    IDSet
		want int
	}
	tests := []testCase{
		{
			name: "empty set",
			s:    NewIDSet(),
			want: 0,
		},
		{
			name: "non-empty set",
			s:    NewIDSet("items", "in", "set"),
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

func TestIDSet_Add(t *testing.T) {
	type args struct {
		ids []ID
	}
	type testCase struct {
		name string
		s    IDSet
		args args
	}
	tests := []testCase{
		{
			name: "add multiple",
			s:    NewIDSet(),
			args: args{ids: []ID{"a", "b", "c"}},
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

func TestIDSet_Remove(t *testing.T) {
	type args struct {
		ids []ID
	}
	type testCase struct {
		name     string
		s        IDSet
		args     args
		expected []ID
	}
	tests := []testCase{
		{
			name:     "remove multiple",
			s:        NewIDSet("a", "b", "c"),
			args:     args{ids: []ID{"a", "b"}},
			expected: []ID{"c"},
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

func TestIDSet_Contains(t *testing.T) {
	type args struct {
		i ID
	}
	type testCase struct {
		name string
		s    IDSet
		args args
		want bool
	}
	tests := []testCase{
		{
			name: "contains",
			s:    NewIDSet("a", "b", "c"),
			args: args{i: "a"},
			want: true,
		},
		{
			name: "not contains",
			s:    NewIDSet("a", "b", "c"),
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

func TestIDSet_Clear(t *testing.T) {
	type testCase struct {
		name string
		s    IDSet
	}
	tests := []testCase{
		{
			name: "go case",
			s:    NewIDSet("a", "b", "c"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.s.Clear()
			assert.Equal(t, 0, tt.s.Size())
		})
	}
}

func TestIDSet_List(t *testing.T) {
	type testCase struct {
		name string
		s    IDSet
		want []ID
	}
	tests := []testCase{
		{
			name: "go case",
			s:    NewIDSet("a", "b", "c"),
			want: []ID{"a", "b", "c"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.ElementsMatchf(t, tt.want, tt.s.List(), "List()")
		})
	}
}

func TestIDSet_Sorted(t *testing.T) {
	type testCase struct {
		name string
		s    IDSet
		want []ID
	}
	tests := []testCase{
		{
			name: "go case",
			s:    NewIDSet("a", "b", "c"),
			want: []ID{"a", "b", "c"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, tt.s.Sorted(), "Sorted()")
		})
	}
}

func TestIDSet_ContainsAny(t *testing.T) {
	type args struct {
		ids []ID
	}
	type testCase struct {
		name string
		s    IDSet
		args args
		want bool
	}
	tests := []testCase{
		{
			name: "contains one",
			s:    NewIDSet("a", "b", "c"),
			args: args{ids: []ID{"a", "x"}},
			want: true,
		},
		{
			name: "contains all",
			s:    NewIDSet("a", "b", "c"),
			args: args{ids: []ID{"a", "b"}},
			want: true,
		},
		{
			name: "contains none",
			s:    NewIDSet("a", "b", "c"),
			args: args{ids: []ID{"x", "y"}},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.s.ContainsAny(tt.args.ids...), fmt.Sprintf("ContainsAny(%v)", tt.args.ids))
		})
	}
}
