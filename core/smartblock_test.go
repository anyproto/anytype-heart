package core

import (
	"reflect"
	"testing"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/textileio/go-threads/core/thread"
)

func TestSmartBlockState_Hash(t *testing.T) {
	tests := []struct {
		name  string
		state SmartBlockState
		want  string
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.Hash(); got != tt.want {
				t.Errorf("SmartBlockState.Hash() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSmartBlockState_VectorCounterPerPeer(t *testing.T) {
	tests := []struct {
		name  string
		state SmartBlockState
		want  map[string]uint64
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.VectorCounterPerPeer(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SmartBlockState.VectorCounterPerPeer() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSmartBlockState_ShouldCreateSnapshot(t *testing.T) {
	tests := []struct {
		name  string
		state SmartBlockState
		want  bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.ShouldCreateSnapshot(); got != tt.want {
				t.Errorf("SmartBlockState.ShouldCreateSnapshot() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMetaChanges_State(t *testing.T) {
	type fields struct {
		Meta  SmartBlockMeta
		state SmartBlockState
	}
	tests := []struct {
		name   string
		fields fields
		want   SmartBlockState
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := &SmartBlockMetaChanges{
				SmartBlockMeta: tt.fields.Meta,
				state:          tt.fields.state,
			}
			if got := meta.State(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SmartBlockMetaChanges.State() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_smartBlock_Creator(t *testing.T) {
	type fields struct {
		thread thread.Info
		node   *Anytype
	}
	tests := []struct {
		name    string
		fields  fields
		want    string
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block := &smartBlock{
				thread: tt.fields.thread,
				node:   tt.fields.node,
			}
			got, err := block.Creator()
			if (err != nil) != tt.wantErr {
				t.Errorf("smartBlock.Creator() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("smartBlock.Creator() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_smartBlock_GetCurrentState(t *testing.T) {
	type fields struct {
		thread thread.Info
		node   *Anytype
	}
	tests := []struct {
		name    string
		fields  fields
		want    SmartBlockState
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block := &smartBlock{
				thread: tt.fields.thread,
				node:   tt.fields.node,
			}
			got, err := block.GetCurrentState()
			if (err != nil) != tt.wantErr {
				t.Errorf("smartBlock.GetCurrentState() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("smartBlock.GetCurrentState() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_smartBlock_PushChanges(t *testing.T) {
	type fields struct {
		thread thread.Info
		node   *Anytype
	}
	type args struct {
		content *SmartBlockContentChanges
		meta    *SmartBlockMetaChanges
	}
	tests := []struct {
		name      string
		fields    fields
		args      args
		wantState SmartBlockState
		wantErr   bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block := &smartBlock{
				thread: tt.fields.thread,
				node:   tt.fields.node,
			}
			gotState, err := block.PushChanges(tt.args.content, tt.args.meta)
			if (err != nil) != tt.wantErr {
				t.Errorf("smartBlock.PushChanges() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotState, tt.wantState) {
				t.Errorf("smartBlock.PushChanges() = %v, want %v", gotState, tt.wantState)
			}
		})
	}
}

func Test_smartBlock_GetThread(t *testing.T) {
	type fields struct {
		thread thread.Info
		node   *Anytype
	}
	tests := []struct {
		name   string
		fields fields
		want   thread.Info
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block := &smartBlock{
				thread: tt.fields.thread,
				node:   tt.fields.node,
			}
			if got := block.GetThread(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("smartBlock.GetThread() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_smartBlock_Type(t *testing.T) {
	type fields struct {
		thread thread.Info
		node   *Anytype
	}
	tests := []struct {
		name   string
		fields fields
		want   SmartBlockType
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block := &smartBlock{
				thread: tt.fields.thread,
				node:   tt.fields.node,
			}
			if got := block.Type(); got != tt.want {
				t.Errorf("smartBlock.Type() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_smartBlock_ID(t *testing.T) {
	type fields struct {
		thread thread.Info
		node   *Anytype
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block := &smartBlock{
				thread: tt.fields.thread,
				node:   tt.fields.node,
			}
			if got := block.ID(); got != tt.want {
				t.Errorf("smartBlock.ID() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_smartBlock_GetLastSnapshot(t *testing.T) {
	type fields struct {
		thread thread.Info
		node   *Anytype
	}
	tests := []struct {
		name    string
		fields  fields
		want    SmartBlockSnapshot
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block := &smartBlock{
				thread: tt.fields.thread,
				node:   tt.fields.node,
			}
			got, err := block.GetLastSnapshot()
			if (err != nil) != tt.wantErr {
				t.Errorf("smartBlock.GetLastSnapshot() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("smartBlock.GetLastSnapshot() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_smartBlock_GetSnapshots(t *testing.T) {
	type fields struct {
		thread thread.Info
		node   *Anytype
	}
	type args struct {
		offset   string
		limit    int
		metaOnly bool
	}
	tests := []struct {
		name          string
		fields        fields
		args          args
		wantSnapshots []smartBlockSnapshot
		wantErr       bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block := &smartBlock{
				thread: tt.fields.thread,
				node:   tt.fields.node,
			}
			gotSnapshots, err := block.GetSnapshots(tt.args.offset, tt.args.limit, tt.args.metaOnly)
			if (err != nil) != tt.wantErr {
				t.Errorf("smartBlock.GetSnapshots() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotSnapshots, tt.wantSnapshots) {
				t.Errorf("smartBlock.GetSnapshots() = %v, want %v", gotSnapshots, tt.wantSnapshots)
			}
		})
	}
}

func Test_smartBlock_PushSnapshot(t *testing.T) {
	type fields struct {
		thread thread.Info
		node   *Anytype
	}
	type args struct {
		state  SmartBlockState
		meta   *SmartBlockMeta
		blocks []*model.Block
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    SmartBlockSnapshot
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block := &smartBlock{
				thread: tt.fields.thread,
				node:   tt.fields.node,
			}
			got, err := block.PushSnapshot(tt.args.state, tt.args.meta, tt.args.blocks)
			if (err != nil) != tt.wantErr {
				t.Errorf("smartBlock.PushSnapshot() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("smartBlock.PushSnapshot() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_smartBlock_EmptySnapshot(t *testing.T) {
	type fields struct {
		thread thread.Info
		node   *Anytype
	}
	tests := []struct {
		name   string
		fields fields
		want   SmartBlockSnapshot
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block := &smartBlock{
				thread: tt.fields.thread,
				node:   tt.fields.node,
			}
			if got := block.EmptySnapshot(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("smartBlock.EmptySnapshot() = %v, want %v", got, tt.want)
			}
		})
	}
}
