package inboxclient

import (
	"context"
	"testing"
)

func TestInboxClient(t *testing.T) {
	ic := New()
	ic.InboxFetch(context.TODO(), "255")
}
