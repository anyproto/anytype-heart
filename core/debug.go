package core

import (
	"context"
	"fmt"
	"time"

	"github.com/anytypeio/go-anytype-middleware/change"
	"github.com/anytypeio/go-anytype-middleware/pb"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/net"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/storage"
	"github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/textileio/go-threads/cbor"
	net3 "github.com/textileio/go-threads/core/net"
	"github.com/textileio/go-threads/core/thread"
)

func (mw *Middleware) DebugSync(req *pb.RpcDebugSyncRequest) *pb.RpcDebugSyncResponse {
	response := func(threads []*pb.RpcDebugSyncResponsethread, threadsWithoutRepl int32, code pb.RpcDebugSyncResponseErrorCode, err error) *pb.RpcDebugSyncResponse {
		m := &pb.RpcDebugSyncResponse{DeviceId: mw.Anytype.Device(), Threads: threads, ThreadsWithoutReplInOwnLog: threadsWithoutRepl, TotalThreads: int32(len(threads)), Error: &pb.RpcDebugSyncResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	var threads []*pb.RpcDebugSyncResponsethread
	t := mw.Anytype.(*core.Anytype).ThreadService().Threads()
	ids, _ := t.Logstore().Threads()
	cafePeer, _ := peer.IDFromString(cafePeerId)
	var threadsWithoutRepl int32
	var threadWithNoHeadDownloaded int32
	for _, id := range ids {
		tinfo := &pb.RpcDebugSyncResponsethread{Id: id.String()}

		thrd, err := t.Logstore().GetThread(id)
		if err != nil {
			threads = append(threads, tinfo)
			log.Errorf("DebugSync failed to getThread: %s", id)
			continue
		}
		for _, lg := range thrd.Logs {
			lgInfo := &pb.RpcDebugSyncResponselog{Id: lg.ID.String(), Head: lg.Head.String()}
			total := 0
			if lg.Head.Defined() {
				rec, rinfo, err := getRecord(t, thrd, lg.Head)
				if rec != nil && err == nil {
					lgInfo.LastRecordTs = int32(rinfo.Time)
					lgInfo.LastRecordVer = int32(rinfo.Version)

					lgInfo.HeadDownloaded = true
					tinfo.LogsWithDownloadedHead++
					rid := lg.Head
					for {
						if !rid.Defined() {
							break
						}
						total++
						if req.RecordsTraverseLimit > 0 && total >= int(req.RecordsTraverseLimit) {
							break
						}
						rec, rinfo, err := getRecord(t, thrd, rid)
						if rec != nil {
							rid = rec.PrevID()
							if !rid.Defined() {
								lgInfo.FirstRecordTs = int32(rinfo.Time)
								lgInfo.FirstRecordVer = int32(rinfo.Version)
								break
							}
						} else {
							log.Errorf("can't continue the traverse, failed to load a record: %s", err.Error())
							break
						}
					}
				}
				lgInfo.TotalRecords = int32(total)
			}
			tinfo.TotalRecords += lgInfo.TotalRecords
			tinfo.Logs = append(tinfo.Logs, lgInfo)
			if lg.ID.String() == mw.Anytype.Device() {
				for _, ad := range lg.Addrs {
					adHost, _ := ad.ValueForProtocol(ma.P_P2P)
					if adHost == cafePeerId {
						tinfo.OwnLogHasCafeReplicator = true
					}
				}
			}
		}
		if tinfo.LogsWithDownloadedHead == 0 {
			threadWithNoHeadDownloaded++
		}

		if !tinfo.OwnLogHasCafeReplicator {
			threadsWithoutRepl++
		}

		ss, err := t.Status(id, cafePeer)
		if err != nil {

		} else {
			if ss.LastPull == 0 {
				tinfo.LastPullSecAgo = -1
			} else {
				tinfo.LastPullSecAgo = int32(time.Now().Unix() - ss.LastPull)
			}
			tinfo.DownStatus = ss.Down.String()
			tinfo.UpStatus = ss.Up.String()
		}

		threads = append(threads, tinfo)
	}
	return response(threads, threadsWithoutRepl, 0, nil)
}

type recordInfo struct {
	Version int
	Time    int64
}

func getRecord(net net.NetBoostrapper, thrd thread.Info, rid cid.Cid) (net3.Record, *recordInfo, error) {
	if thrd.ID == thread.Undef {
		return nil, nil, fmt.Errorf("undef id")
	}

	hasBlock, err := net.GetIpfs().HasBlock(rid)
	if err != nil {
		return nil, nil, err
	}
	if !hasBlock {
		return nil, nil, fmt.Errorf("doesn't have locally")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()

	rec, err := net.GetRecord(ctx, thrd.ID, rid)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load record: %s", err.Error())
	}

	event, err := cbor.EventFromRecord(ctx, net, rec)
	if err != nil {
		return nil, nil, err
	}

	node, err := event.GetBody(context.TODO(), net, thrd.Key.Read())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get record body: %w", err)
	}
	m := new(core.SignedPbPayload)
	err = cbornode.DecodeInto(node.RawData(), m)
	if err != nil {
		return nil, nil, fmt.Errorf("cbor decode error: %w", err)
	}

	var ts int64
	err = m.Verify()
	if err != nil {
		return nil, nil, err
	}

	if m.Ver > 0 {
		sbe := core.SmartblockRecordEnvelope{SmartblockRecord: core.SmartblockRecord{ID: rid.String(), PrevID: rec.PrevID().String(), Payload: m.Data}}
		ch, _ := change.NewChangeFromRecord(sbe)
		if ch != nil {
			ts = ch.Timestamp
		}
	} else {
		var snapshot = storage.SmartBlockSnapshot{}
		err = m.Unmarshal(&snapshot)
		if err == nil {
			ts = snapshot.ClientTime
		}
	}

	return rec, &recordInfo{Version: int(m.Ver), Time: ts}, nil
}
