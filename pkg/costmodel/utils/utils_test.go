package utils

import (
	"testing"
)

func TestBytesToGiB(t *testing.T) {
	type args struct {
		bytes int64
	}
	tests := []struct {
		name string
		args args
		want float64
	}{
		{
			name: "0 bytes",
			args: args{
				bytes: 0,
			},
			want: 0,
		},
		{
			name: "1 GiB",
			args: args{
				bytes: 1073741824,
			},
			want: 1,
		},
		{
			name: "2 GiB",
			args: args{
				bytes: 2147483648,
			},
			want: 2,
		},
		{
			name: "Half a GiB",
			args: args{
				bytes: 536870912,
			},
			want: 0.5,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BytesToGiB(tt.args.bytes); got != tt.want {
				t.Errorf("BytesToGiB() = %v, want %v", got, tt.want)
			}
		})
	}
}
