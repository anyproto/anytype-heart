package invitecleanup

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func detailsSet(key, value string) *pb.ChangeContent {
	return &pb.ChangeContent{
		Value: &pb.ChangeContentValueOfDetailsSet{
			DetailsSet: &pb.ChangeDetailsSet{
				Key:   key,
				Value: &types.Value{Kind: &types.Value_StringValue{StringValue: value}},
			},
		},
	}
}

func detailsUnset(key string) *pb.ChangeContent {
	return &pb.ChangeContent{
		Value: &pb.ChangeContentValueOfDetailsUnset{
			DetailsUnset: &pb.ChangeDetailsUnset{Key: key},
		},
	}
}

// anyoneInvite is what SetInviteFileInfo produces: the cid and the key in a single Apply, so in a
// single change. An invite never gets one without the other.
func anyoneInvite(cid, key string) *pb.Change {
	return &pb.Change{Content: []*pb.ChangeContent{
		detailsSet(bundle.RelationKeySpaceInviteFileCid.String(), cid),
		detailsSet(bundle.RelationKeySpaceInviteFileKey.String(), key),
	}}
}

func inviteSnapshot(details map[string]string) *pb.Change {
	fields := map[string]*types.Value{}
	for k, v := range details {
		fields[k] = &types.Value{Kind: &types.Value_StringValue{StringValue: v}}
	}
	return &pb.Change{Snapshot: &pb.ChangeSnapshot{
		Data: &model.SmartBlockSnapshotBase{Details: &types.Struct{Fields: fields}},
	}}
}

func collect(t *testing.T, changes []*pb.Change) []inviteFile {
	t.Helper()
	collector := newInviteFileCollector()
	for _, change := range changes {
		collector.addChange(change)
	}
	return collector.result()
}

func TestInviteFileCollector(t *testing.T) {
	t.Run("every invite the workspace ever had, the current one included", func(t *testing.T) {
		// given three invites generated and revoked in turn, the last one still current. The current
		// one is a candidate like any other: whether it is live is for the acl to say, and a detail
		// that may be behind is no basis for dropping a file from the scan.
		changes := []*pb.Change{
			anyoneInvite("cid1", "key1"),
			{Content: []*pb.ChangeContent{detailsUnset(bundle.RelationKeySpaceInviteFileCid.String())}},
			anyoneInvite("cid2", "key2"),
			anyoneInvite("cid3", "key3"),
		}
		want := []inviteFile{
			{cid: "cid1", key: "key1"},
			{cid: "cid2", key: "key2"},
			{cid: "cid3", key: "key3"},
		}

		got := collect(t, changes)

		assert.Equal(t, want, got)
	})

	t.Run("invite recorded only in a snapshot", func(t *testing.T) {
		// given a snapshot taken while an invite was live, its original change compacted away
		changes := []*pb.Change{
			inviteSnapshot(map[string]string{
				bundle.RelationKeySpaceInviteFileCid.String(): "cid1",
				bundle.RelationKeySpaceInviteFileKey.String(): "key1",
			}),
		}
		want := []inviteFile{{cid: "cid1", key: "key1"}}

		got := collect(t, changes)

		assert.Equal(t, want, got)
	})

	t.Run("guest invite is never a candidate", func(t *testing.T) {
		// given a guest invite file, whose key still grants read access
		changes := []*pb.Change{
			{Content: []*pb.ChangeContent{
				detailsSet(bundle.RelationKeySpaceInviteGuestFileCid.String(), "guestCid"),
				detailsSet(bundle.RelationKeySpaceInviteGuestFileKey.String(), "guestKey"),
			}},
			// the same cid also showing up as a regular invite must not resurrect it
			anyoneInvite("guestCid", "guestKey"),
		}

		got := collect(t, changes)

		assert.Empty(t, got)
	})

	t.Run("cid without a key is dropped", func(t *testing.T) {
		// given a cid whose key never made it into the history: the coordinator cannot decrypt the
		// file without it, so there is nothing to be done with the cid
		changes := []*pb.Change{
			{Content: []*pb.ChangeContent{detailsSet(bundle.RelationKeySpaceInviteFileCid.String(), "cid1")}},
		}

		got := collect(t, changes)

		assert.Empty(t, got)
	})

	t.Run("repeated cid is reported once", func(t *testing.T) {
		// given the same invite in a snapshot and in the change that created it
		changes := []*pb.Change{
			anyoneInvite("cid1", "key1"),
			inviteSnapshot(map[string]string{
				bundle.RelationKeySpaceInviteFileCid.String(): "cid1",
				bundle.RelationKeySpaceInviteFileKey.String(): "key1",
			}),
		}
		want := []inviteFile{{cid: "cid1", key: "key1"}}

		got := collect(t, changes)

		assert.Equal(t, want, got)
	})

	t.Run("workspace without invites", func(t *testing.T) {
		changes := []*pb.Change{
			{Content: []*pb.ChangeContent{detailsSet(bundle.RelationKeyName.String(), "space")}},
			inviteSnapshot(map[string]string{bundle.RelationKeyName.String(): "space"}),
		}

		got := collect(t, changes)

		assert.Empty(t, got)
	})
}
