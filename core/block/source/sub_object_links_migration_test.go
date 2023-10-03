package source

import (
	"testing"

	"github.com/anyproto/anytype-heart/core/domain"
)

func TestSubObjectIdToUniqueKey(t *testing.T) {
	type args struct {
		id string
	}
	tests := []struct {
		name      string
		args      args
		wantUk    string
		wantValid bool
	}{
		{"relation", args{"rel-id"}, "rel-id", true},
		{"type", args{"ot-task"}, "ot-task", true},
		{"opt", args{"650832666293ae9ae67e5f9c"}, "opt-650832666293ae9ae67e5f9c", true},
		{"invalid-prefix", args{"aa-task"}, "", false},
		{"no-key", args{"rel"}, "", false},
		{"no-key2", args{"rel-"}, "", false},
		{"no-key2", args{"rel---gdfgfd--gfdgfd-"}, "", false},
		{"invalid", args{"task"}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotUk, gotValid := subObjectIdToUniqueKey(tt.args.id)
			if gotValid != tt.wantValid {
				t.Errorf("SubObjectIdToUniqueKey() gotValid = %v, want %v", gotValid, tt.wantValid)
				t.Fail()
			}

			if !tt.wantValid {
				return
			}

			wantUk, err := domain.UnmarshalUniqueKey(tt.wantUk)
			if err != nil {
				t.Errorf("SubObjectIdToUniqueKey() error = %v", err)
				t.Fail()
			}
			if wantUk.Marshal() != gotUk.Marshal() {
				t.Errorf("SubObjectIdToUniqueKey() gotUk = %v, want %v", gotUk, tt.wantUk)
				t.Fail()
			}
			if wantUk.SmartblockType() != gotUk.SmartblockType() {
				t.Errorf("SubObjectIdToUniqueKey() gotSmartblockType = %v, want %v", gotUk.SmartblockType(), wantUk.SmartblockType())
				t.Fail()
			}
		})
	}
}
