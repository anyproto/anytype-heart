package invitecleanup

import (
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

// inviteFile is an invite file the workspace object pointed at during its lifetime.
type inviteFile struct {
	cid string
	key string
}

// inviteFileCollector reconstructs every invite file a space ever had from the change history of its
// workspace object.
//
// SetInviteFileInfo writes the cid and the key in a single Apply, and an invite never gets one
// without the other, so they always land in the same change. A snapshot taken while an invite was
// live carries the pair in its details and is the only surviving record of that invite once the
// original change is compacted away, so both sources have to be scanned.
//
// Guest invite files are collected separately: a guest invite is never revoked, so its file stays.
type inviteFileCollector struct {
	files     []inviteFile
	seen      map[string]struct{}
	guestCids map[string]struct{}
}

func newInviteFileCollector() *inviteFileCollector {
	return &inviteFileCollector{
		seen:      map[string]struct{}{},
		guestCids: map[string]struct{}{},
	}
}

func (c *inviteFileCollector) addChange(ch *pb.Change) {
	if ch == nil {
		return
	}
	if snapshot := ch.GetSnapshot(); snapshot != nil && snapshot.Data != nil {
		fields := snapshot.Data.Details.GetFields()
		c.add(
			fields[bundle.RelationKeySpaceInviteFileCid.String()].GetStringValue(),
			fields[bundle.RelationKeySpaceInviteFileKey.String()].GetStringValue(),
		)
		c.addGuest(fields[bundle.RelationKeySpaceInviteGuestFileCid.String()].GetStringValue())
	}

	var cid, key string
	for _, content := range ch.Content {
		set := content.GetDetailsSet()
		if set == nil {
			continue
		}
		switch set.Key {
		case bundle.RelationKeySpaceInviteFileCid.String():
			cid = set.Value.GetStringValue()
		case bundle.RelationKeySpaceInviteFileKey.String():
			key = set.Value.GetStringValue()
		case bundle.RelationKeySpaceInviteGuestFileCid.String():
			c.addGuest(set.Value.GetStringValue())
		}
	}
	c.add(cid, key)
}

// add records a pair. A cid without its key is dropped: the invite file cannot be read without it,
// so nothing could be established about the invite and its file must stay where it is.
func (c *inviteFileCollector) add(cid, key string) {
	if cid == "" || key == "" {
		return
	}
	if _, ok := c.seen[cid]; ok {
		return
	}
	c.seen[cid] = struct{}{}
	c.files = append(c.files, inviteFile{cid: cid, key: key})
}

func (c *inviteFileCollector) addGuest(cid string) {
	if cid != "" {
		c.guestCids[cid] = struct{}{}
	}
}

// result returns every invite file found in the history, guest invites aside. The invite that is
// current is not filtered out here: whether an invite is still in use is for the acl to say, and a
// detail that may be behind is no basis for dropping a file from the scan.
func (c *inviteFileCollector) result() []inviteFile {
	var out []inviteFile
	for _, f := range c.files {
		if _, ok := c.guestCids[f.cid]; ok {
			continue
		}
		out = append(out, f)
	}
	return out
}
