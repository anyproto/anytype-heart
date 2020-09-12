package wallet

import (
	"reflect"
	"testing"
)

func TestNewPubKeyFromAddress(t *testing.T) {
	type args struct {
		t       KeypairType
		address string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "account",
			args: args{
				t:       KeypairTypeAccount,
				address: "AAYQRgKRrhD2Fv8t3nKpBY35A7PwiJJXrzJeHJ2N3uBU3iAi",
			},
			wantErr: false,
		},
		{
			name: "device",
			args: args{
				t:       KeypairTypeDevice,
				address: "12D3KooWRPgmQSbw7Kunskzb4DUXzC3opcCbZxts7sRitbbY6LEE",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewPubKeyFromAddress(tt.args.t, tt.args.address)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewPubKeyFromAddress() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got.Address(), tt.args.address) {
				t.Errorf("NewPubKeyFromAddress() = %v, want %v", got.Address(), tt.args.address)
			}
		})
	}
}
