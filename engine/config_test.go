package engine

import (
	"reflect"
	"testing"

	"golang.org/x/time/rate"
)

func Test_rateLimiter(t *testing.T) {
	type args struct {
		rstr string
	}
	tests := []struct {
		name    string
		args    args
		want    *rate.Limiter
		wantErr bool
	}{
		{"low", args{"LOW"}, rate.NewLimiter(rate.Limit(50000), 50000*3), false},
		{"case", args{"LoW"}, rate.NewLimiter(rate.Limit(50000), 50000*3), false},
		{"err", args{"fake"}, nil, true},
		{"unit", args{"10kb"}, rate.NewLimiter(rate.Limit(10240), 10240*3), false},
		{"unit", args{"100kb"}, rate.NewLimiter(rate.Limit(102400), 102400*3), false},
		{"unit", args{"100 kb"}, rate.NewLimiter(rate.Limit(102400), 102400*3), false},
		{"inf", args{"0"}, rate.NewLimiter(rate.Inf, 0), false},
		{"inf", args{""}, rate.NewLimiter(rate.Inf, 0), false},

		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := rateLimiter(tt.args.rstr)
			if (err != nil) != tt.wantErr {
				t.Errorf("rateLimiter() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("rateLimiter() = %v, want %v", got, tt.want)
			}
		})
	}
}
