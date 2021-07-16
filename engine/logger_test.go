package engine

import (
	"reflect"
	"testing"
)

func Test_filteredLogger_filteredArg(t *testing.T) {
	type args struct {
		v []interface{}
	}
	tests := []struct {
		name string
		args args
		want []interface{}
	}{
		// TODO: Add test cases.
		{"1", args{v: []interface{}{"123"}}, []interface{}{"123"}},
		{"2", args{v: []interface{}{"abcdef1234567890abcdef1234567890abcdef12"}}, []interface{}{"[abcdef..]"}},
		{"3", args{v: []interface{}{"abcdef1234567890abcdef1234567890abcdef12", "123"}}, []interface{}{"[abcdef..]", "123"}},
		{"4", args{v: []interface{}{taskType(1), taskType(0)}}, []interface{}{"[Magnet]", "[Torrent]"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := log.filteredArg(tt.args.v...); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("filteredLogger.filteredArg() = %v, want %v", got, tt.want)
			}
		})
	}
}
