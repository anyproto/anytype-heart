package core

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/protobuf/ptypes"
	mh "github.com/multiformats/go-multihash"
	"github.com/textileio/go-textile/broadcast"
	"github.com/textileio/go-textile/core"
	tpb "github.com/textileio/go-textile/pb"
)

func (a *Anytype) AccountSetName(username string) error {
	return a.Textile.SetName(username)
}

func (a *Anytype) AccountSetAvatar(localPath string) (hash mh.Multihash, err error) {
	if !a.Textile.Online() {
		return nil, core.ErrOffline
	}

	thrd := a.Textile.Node().AccountThread()
	if thrd == nil {
		return nil, fmt.Errorf("account thread not found")
	}

	hash, err = a.Textile.AddFilesSync([]string{localPath}, thrd.Id, "")
	if err != nil {
		return nil, err
	}

	err = a.Textile.Node().SetAvatar()
	if err != nil {
		return nil, err
	}

	a.Textile.Node().FlushCafes()

	return hash, nil
}

func (a *Anytype) AccountRequestStoredContact(ctx context.Context, accountId string) (contact *tpb.Contact, err error) {
	contact = a.Textile.Node().Contact(accountId)

	if contact != nil && (contact.Name != "" || contact.Avatar != ""){
		return contact, nil
	}
	// reset in case local contact wasn't full
	contact = nil

	var resCh <-chan *tpb.QueryResult
	var errCh <-chan error
	var cancel *broadcast.Broadcaster
	resCh, errCh, cancel, err = a.Textile.Node().SearchContacts(&tpb.ContactQuery{Address: accountId}, &tpb.QueryOptions{
		Wait: 5,
	})

	if err != nil {
		return
	}

	readTimeout := time.After(time.Second * 30)
	for {
		select {
		case <-ctx.Done():
			cancel.Close()
			err = fmt.Errorf("read timeout")
			return
		case <-readTimeout:
			// this was introduced because we doesn't use pubsub to query this (only cafe api)
			// so all results will come in one batch
			cancel.Close()
			return
		case err = <-errCh:
			return
		case res, ok := <-resCh:
			if !ok {
				return
			}
			contact = &tpb.Contact{}
			err = ptypes.UnmarshalAny(res.Value, contact)
			if err != nil {
				return
			}
			// reset readTimeout
			readTimeout = time.After(time.Second)
		}
	}

}
