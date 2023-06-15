package file

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
			s:    NewIDSet(1, 2, 3),
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
			args: args{ids: []ID{1, 2, 3}},
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
			s:        NewIDSet(1, 2, 3),
			args:     args{ids: []ID{1, 2}},
			expected: []ID{3},
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
			s:    NewIDSet(1, 2, 3),
			args: args{i: 1},
			want: true,
		},
		{
			name: "not contains",
			s:    NewIDSet(1, 2, 3),
			args: args{i: 97},
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
			s:    NewIDSet(1, 2, 3),
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
			s:    NewIDSet(1, 2, 3),
			want: []ID{1, 2, 3},
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
			s:    NewIDSet(1, 2, 3),
			want: []ID{1, 2, 3},
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
			s:    NewIDSet(1, 2, 3),
			args: args{ids: []ID{1, 97}},
			want: true,
		},
		{
			name: "contains all",
			s:    NewIDSet(1, 2, 3),
			args: args{ids: []ID{1, 2}},
			want: true,
		},
		{
			name: "contains none",
			s:    NewIDSet(1, 2, 3),
			args: args{ids: []ID{97, 98}},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.s.ContainsAny(tt.args.ids...), fmt.Sprintf("ContainsAny(%v)", tt.args.ids))
		})
	}
}
