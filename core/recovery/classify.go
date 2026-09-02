package recovery

import (
	"context"
	"errors"
	gonet "net"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"github.com/anyproto/any-sync/net"
	"github.com/anyproto/any-sync/net/peerservice"
	"github.com/anyproto/any-sync/net/secureservice/handshake"

	"github.com/anyproto/anytype-heart/pb"
)

// debugMessageMax bounds the raw error text carried for logs.
const debugMessageMax = 256

// errInfo is the classified form of an error: the wire class, whether a retry
// can help, and the raw text for diagnostics.
type errInfo struct {
	class     pb.EventAccountRecoveryErrorClass
	retryable bool
	debug     string
}

func (e *errInfo) toPb() *pb.EventAccountRecoveryErrorInfo {
	if e == nil {
		return nil
	}
	return &pb.EventAccountRecoveryErrorInfo{
		Class:        e.class,
		Retryable:    e.retryable,
		DebugMessage: e.debug,
	}
}

// classify maps an error onto the shared ErrorClass taxonomy. errors.Is is
// used throughout, so a joined dial error (errors.Join of every per-address
// failure) is classified by its best member: the checks run in specificity
// order and the first match wins, so a version mismatch on one address
// outranks a timeout on another. A nil error and a cancellation classify to
// nil: neither is a failure.
func classify(err error) *errInfo {
	if err == nil || errors.Is(err, context.Canceled) {
		return nil
	}
	info := &errInfo{debug: truncate(err.Error(), debugMessageMax)}
	switch {
	case errors.Is(err, handshake.ErrIncompatibleVersion),
		errors.Is(err, handshake.ErrIncompatibleProto),
		errors.Is(err, handshake.ErrRemoteIncompatibleProto):
		info.class = pb.EventAccountRecovery_IncompatibleVersion
	case errors.Is(err, handshake.ErrInvalidCredentials),
		errors.Is(err, handshake.ErrPeerDeclinedCredentials),
		errors.Is(err, handshake.ErrSkipVerifyNotAllowed),
		errors.Is(err, coordinatorproto.ErrForbidden):
		info.class = pb.EventAccountRecovery_NotAuthorized
	case errors.Is(err, coordinatorproto.ErrAccountIsDeleted):
		info.class = pb.EventAccountRecovery_AccountDeleted
	case errors.Is(err, spacesyncproto.ErrSpaceIsDeleted),
		errors.Is(err, coordinatorproto.ErrSpaceIsDeleted),
		errors.Is(err, coordinatorproto.ErrSpaceDeletionPending):
		info.class = pb.EventAccountRecovery_SpaceDeleted
	case errors.Is(err, spacesyncproto.ErrTooManyRequestsFromPeer),
		errors.Is(err, spacesyncproto.ErrDuplicateRequest):
		info.class, info.retryable = pb.EventAccountRecovery_RateLimited, true
	case errors.Is(err, net.ErrUnableToConnect),
		errors.Is(err, peerservice.ErrAddrsNotFound),
		errors.Is(err, handshake.ErrDeadlineExceeded),
		errors.Is(err, context.DeadlineExceeded),
		errors.Is(err, spacesyncproto.ErrPeerIsNotResponsible),
		isNetworkError(err):
		info.class, info.retryable = pb.EventAccountRecovery_PeerUnreachable, true
	case errors.Is(err, objecttree.ErrHasInvalidChanges),
		errors.Is(err, anystore.ErrCollectionNotFound):
		info.class = pb.EventAccountRecovery_Unexpected
	default:
		info.class, info.retryable = pb.EventAccountRecovery_Unexpected, true
	}
	return info
}

// classifyAccount is classify in the tech-space context, where a deleted or
// missing space means the account itself is deleted or not on this network.
func classifyAccount(err error) *errInfo {
	info := classify(err)
	if info == nil {
		return nil
	}
	switch {
	case info.class == pb.EventAccountRecovery_SpaceDeleted:
		info.class = pb.EventAccountRecovery_AccountDeleted
	case info.class == pb.EventAccountRecovery_Unexpected && errors.Is(err, spacesyncproto.ErrSpaceMissing):
		info.class, info.retryable = pb.EventAccountRecovery_AccountNotFound, false
	}
	return info
}

// isNetworkError covers transport-level failures that carry no sentinel: a
// *net.OpError, or any net.Error reporting a timeout (which is how quic-go's
// handshake and idle timeouts surface without a direct quic-go import).
func isNetworkError(err error) bool {
	var opErr *gonet.OpError
	if errors.As(err, &opErr) {
		return true
	}
	var netErr gonet.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
