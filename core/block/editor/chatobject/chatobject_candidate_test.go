package chatobject

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
)

type fakeUnreadRepo struct {
	gotCounter chatmodel.CounterType
	ids        []string
	err        error
}

func (f *fakeUnreadRepo) GetAllUnreadMessages(_ context.Context, c chatmodel.CounterType) ([]string, error) {
	f.gotCounter = c
	return f.ids, f.err
}

func TestUnreadCandidateProvider_DelegatesPerCounter(t *testing.T) {
	repo := &fakeUnreadRepo{ids: []string{"a", "b"}}
	s := &storeObject{}
	p := s.unreadCandidateProviderFromFn(repo.GetAllUnreadMessages, chatmodel.CounterTypeMention)

	got, err := p(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, got)
	assert.Equal(t, chatmodel.CounterTypeMention, repo.gotCounter)

	repo2 := &fakeUnreadRepo{err: errors.New("boom")}
	p2 := s.unreadCandidateProviderFromFn(repo2.GetAllUnreadMessages, chatmodel.CounterTypeMessage)
	_, err = p2(context.Background())
	assert.Error(t, err)
}
