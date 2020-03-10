package core

import (
	"reflect"
	"sync"
	"testing"

	"github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p/p2p/discovery"
	"github.com/stretchr/testify/require"
	"github.com/textileio/go-textile/keypair"
	"github.com/textileio/go-threads/store"
)

var doOnce sync.Once
var s Service

func getRunningService(t *testing.T) Service{
	doOnce.Do(func(){
		s = createAccount(t)
		err := s.Start()
		require.NoError(t, err)
	})
	return s
}

func TestAnytype_CreateBlock(t *testing.T) {
	type fields struct {
		ds                 datastore.Batching
		repoPath           string
		ts                 store.ServiceBoostrapper
		mdns               discovery.Service
		account            *keypair.Full
		predefinedBlockIds PredefinedBlockIds
		logLevels          map[string]string
		lock               sync.Mutex
		done               chan struct{}
	}
	type args struct {
		t SmartBlockType
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    SmartBlock
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Anytype{
				ds:                 tt.fields.ds,
				repoPath:           tt.fields.repoPath,
				ts:                 tt.fields.ts,
				mdns:               tt.fields.mdns,
				account:            tt.fields.account,
				predefinedBlockIds: tt.fields.predefinedBlockIds,
				logLevels:          tt.fields.logLevels,
				lock:               tt.fields.lock,
				done:               tt.fields.done,
			}
			got, err := a.CreateBlock(tt.args.t)
			if (err != nil) != tt.wantErr {
				t.Errorf("Anytype.CreateBlock() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Anytype.CreateBlock() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAnytype_IsStarted(t *testing.T) {
	s := getRunningService(t)
	require.True(t, s.IsStarted())
}

func TestAnytype_PredefinedBlocks(t *testing.T) {
	s := getRunningService(t)
	require.NotNil(t, s)
	require.Len(t, s.PredefinedBlocks().Home,57)
}
