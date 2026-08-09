package modelsync

import (
	"reflect"
	"testing"
)

func TestDedupeModels(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "无重复",
			in:   []string{"gpt-4o", "gpt-4o-mini", "claude-3"},
			want: []string{"gpt-4o", "gpt-4o-mini", "claude-3"},
		},
		{
			name: "有重复保留首个",
			in:   []string{"gpt-4o", "claude-3", "gpt-4o", "claude-3", "gpt-4o"},
			want: []string{"gpt-4o", "claude-3"},
		},
		{
			name: "全部相同",
			in:   []string{"gpt-4o", "gpt-4o", "gpt-4o"},
			want: []string{"gpt-4o"},
		},
		{
			name: "空列表",
			in:   []string{},
			want: []string{},
		},
		{
			name: "nil 列表",
			in:   nil,
			want: []string{},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := dedupeModels(c.in)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("dedupeModels(%v) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}
