package engine

import (
	"testing"
)

func Test_filteredLogger_Println(t *testing.T) {
	type args struct {
		v []interface{}
	}
	tests := []struct {
		name string
		args args
	}{
		{
			"1", args{v: []interface{}{"1", "shoud hide", "abcdef1234567890abcdef1234567890abcdef12"}},
		},
		{

			"2", args{v: []interface{}{"2", "shoud not hide", "1abcdef1234567890abcdef1234567890abcdef12"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log.Println(tt.args.v...)
		})
	}
}
