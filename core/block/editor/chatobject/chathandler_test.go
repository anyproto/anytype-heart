package chatobject

import (
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/stretchr/testify/require"
)

// TestCanModerateAt drives ChatHandler.canModerateAt against a real ACL list
// built via list.NewAclExecutor. Owners and Admins should be permitted to
// moderate (e.g. delete others' messages); writers and readers should not.
func TestCanModerateAt(t *testing.T) {
	exec := list.NewAclExecutor("space")
	cmds := []struct {
		cmd string
		err error
	}{
		{"a.init::a", nil},
		{"a.invite::invId", nil},
		{"adm.join::invId", nil},
		{"a.approve::adm,adm", nil},
		{"writer.join::invId", nil},
		{"a.approve::writer,rw", nil},
		{"reader.join::invId", nil},
		{"a.approve::reader,r", nil},
	}
	for _, c := range cmds {
		require.Equal(t, c.err, exec.Execute(c.cmd), c.cmd)
	}

	acl := exec.ActualAccounts()["a"].Acl
	headId := acl.Head().Id

	ownerAccount := exec.ActualAccounts()["a"].Keys.SignKey.GetPublic().Account()
	adminAccount := exec.ActualAccounts()["adm"].Keys.SignKey.GetPublic().Account()
	writerAccount := exec.ActualAccounts()["writer"].Keys.SignKey.GetPublic().Account()
	readerAccount := exec.ActualAccounts()["reader"].Keys.SignKey.GetPublic().Account()

	h := &ChatHandler{aclList: acl}

	t.Run("owner can moderate", func(t *testing.T) {
		require.True(t, h.canModerateAt(ownerAccount, headId))
	})
	t.Run("admin can moderate", func(t *testing.T) {
		require.True(t, h.canModerateAt(adminAccount, headId))
	})
	t.Run("writer cannot moderate", func(t *testing.T) {
		require.False(t, h.canModerateAt(writerAccount, headId))
	})
	t.Run("reader cannot moderate", func(t *testing.T) {
		require.False(t, h.canModerateAt(readerAccount, headId))
	})
	t.Run("nil acl falls back to false", func(t *testing.T) {
		empty := &ChatHandler{}
		require.False(t, empty.canModerateAt(ownerAccount, headId))
	})
	t.Run("empty aclHeadId falls back to false", func(t *testing.T) {
		require.False(t, h.canModerateAt(ownerAccount, ""))
	})
	t.Run("garbage account falls back to false", func(t *testing.T) {
		require.False(t, h.canModerateAt("not-a-valid-account", headId))
	})
}
