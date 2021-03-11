package core

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/textileio/go-threads/jsonpatcher"

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

func (mw *Middleware) DebugThread(req *pb.RpcDebugThreadRequest) *pb.RpcDebugThreadResponse {
	response := func(thread *pb.RpcDebugthreadInfo, code pb.RpcDebugThreadResponseErrorCode, err error) *pb.RpcDebugThreadResponse {
		m := &pb.RpcDebugThreadResponse{Info: thread, Error: &pb.RpcDebugThreadResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	mw.m.RLock()
	defer mw.m.RUnlock()
	if mw.app == nil {
		return response(nil, 0, nil)
	}

	at := mw.app.MustComponent(core.CName).(core.Service)

	t := at.(*core.Anytype).ThreadService().Threads()
	cafePeer, _ := peer.Decode(cafePeerId)

	tid, err := thread.Decode(req.ThreadId)
	if err != nil {
		return response(nil, pb.RpcDebugThreadResponseError_BAD_INPUT, err)
	}

	tinfo := getThreadInfo(t, tid, at.Device(), cafePeer, req.SkipEmptyLogs, req.TryToDownloadRemoteRecords)
	return response(&tinfo, 0, nil)
}

func (mw *Middleware) DebugSync(req *pb.RpcDebugSyncRequest) *pb.RpcDebugSyncResponse {
	mw.m.RLock()
	if mw.app == nil {
		return &pb.RpcDebugSyncResponse{}
	}
	at := mw.app.MustComponent(core.CName).(core.Service)
	mw.m.Unlock()

	response := func(threads []*pb.RpcDebugthreadInfo, threadsWithoutRepl int32, threadsWithoutHeadDownloaded int32, totalRecords int32, totalSize int32, code pb.RpcDebugSyncResponseErrorCode, err error) *pb.RpcDebugSyncResponse {
		m := &pb.RpcDebugSyncResponse{DeviceId: at.Device(), Threads: threads, ThreadsWithoutReplInOwnLog: threadsWithoutRepl, ThreadsWithoutHeadDownloaded: threadsWithoutHeadDownloaded, TotalThreads: int32(len(threads)), TotalRecords: totalRecords, TotalSize: totalSize, Error: &pb.RpcDebugSyncResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	var threads []*pb.RpcDebugthreadInfo
	t := at.(*core.Anytype).ThreadService().Threads()
	ids, _ := t.Logstore().Threads()
	cafePeer, _ := peer.Decode(cafePeerId)

	var (
		threadsWithoutRepl         int32
		threadWithNoHeadDownloaded int32
		totalRecords               int32
		totalSize                  int32
	)

	for _, id := range ids {
		tinfo := getThreadInfo(t, id, at.Device(), cafePeer, req.SkipEmptyLogs, req.TryToDownloadRemoteRecords)
		if tinfo.LogsWithDownloadedHead == 0 {
			threadWithNoHeadDownloaded++
		}

		if !tinfo.OwnLogHasCafeReplicator {
			threadsWithoutRepl++
		}

		totalRecords += tinfo.TotalRecords
		totalSize += tinfo.TotalSize

		threads = append(threads, &tinfo)
	}

	sort.Slice(threads, func(i, j int) bool {
		return threads[i].Id < threads[j].Id
	})

	return response(threads, threadsWithoutRepl, threadWithNoHeadDownloaded, totalRecords, totalSize, 0, nil)
}

func getThreadInfo(t net.NetBoostrapper, id thread.ID, ownDeviceId string, cafePeer peer.ID, skipEmptyLogs bool, downloadRemoteRecords bool) pb.RpcDebugthreadInfo {
	tinfo := pb.RpcDebugthreadInfo{Id: id.String()}
	thrd, err := t.Logstore().GetThread(id)
	if err != nil {
		log.Errorf("DebugSync failed to getThread: %s", id)
		tinfo.Error = err.Error()
		return tinfo
	}
	for _, lg := range thrd.Logs {
		lgInfo := getLogInfo(t, thrd, lg, downloadRemoteRecords, 0)
		if skipEmptyLogs && len(lgInfo.Head) <= 1 {
			continue
		}

		tinfo.TotalRecords += lgInfo.TotalRecords
		tinfo.Logs = append(tinfo.Logs, &lgInfo)
		tinfo.TotalSize += lgInfo.TotalSize
		if lgInfo.FirstRecordTs > 0 {
			tinfo.LogsWithWholeTreeDownloaded++
		}

		if lg.ID.String() == ownDeviceId {
			for _, ad := range lg.Addrs {
				adHost, _ := ad.ValueForProtocol(ma.P_P2P)
				if adHost == cafePeerId {
					tinfo.OwnLogHasCafeReplicator = true
				}
			}
		}
		if lgInfo.HeadDownloaded {
			tinfo.LogsWithDownloadedHead++
		}
	}

	sort.Slice(tinfo.Logs, func(i, j int) bool {
		return tinfo.Logs[i].Id < tinfo.Logs[j].Id
	})

	ss, err := t.Status(id, cafePeer)
	if err != nil {
		tinfo.CafeDownStatus = err.Error()
		tinfo.CafeUpStatus = err.Error()
		tinfo.CafeLastPullSecAgo = -1
	} else {
		if ss.LastPull == 0 {
			tinfo.CafeLastPullSecAgo = -1
		} else {
			tinfo.CafeLastPullSecAgo = int32(time.Now().Unix() - ss.LastPull)
		}
		tinfo.CafeDownStatus = ss.Down.String()
		tinfo.CafeUpStatus = ss.Up.String()
	}
	return tinfo
}

func getLogInfo(t net.NetBoostrapper, thrd thread.Info, lg thread.LogInfo, downloadRemote bool, maxRecords int) pb.RpcDebuglogInfo {
	lgInfo := pb.RpcDebuglogInfo{Id: lg.ID.String(), Head: lg.Head.String()}
	if !lg.Head.Defined() {
		return lgInfo
	}

	rec, rinfo, err := getRecord(t, thrd, lg.Head, downloadRemote)
	if rec != nil && err == nil {
		lgInfo.LastRecordTs = int32(rinfo.Time)
		lgInfo.LastRecordVer = int32(rinfo.Version)

		lgInfo.HeadDownloaded = true
		rid := lg.Head
		for {
			if !rid.Defined() {
				break
			}
			lgInfo.TotalRecords++
			if maxRecords > 0 && lgInfo.TotalRecords >= int32(maxRecords) {
				break
			}
			rec, rinfo, err := getRecord(t, thrd, rid, downloadRemote)
			if rec != nil {
				lgInfo.TotalSize += int32(rinfo.Size)
				rid = rec.PrevID()
				if !rid.Defined() {
					lgInfo.FirstRecordTs = int32(rinfo.Time)
					lgInfo.FirstRecordVer = int32(rinfo.Version)
					break
				}
			} else {
				if err != nil {
					lgInfo.Error = err.Error()
					log.Errorf("can't continue the traverse, failed to load a record: %s", err.Error())
				}
				break
			}
		}
	} else if err != nil {
		lgInfo.Error = err.Error()
	}

	ss, err := t.Status(thrd.ID, lg.ID)
	if err != nil {
		lgInfo.DownStatus = err.Error()
		lgInfo.UpStatus = err.Error()
		lgInfo.LastPullSecAgo = -1
	} else {
		if ss.LastPull == 0 {
			lgInfo.LastPullSecAgo = -1
		} else {
			lgInfo.LastPullSecAgo = int32(time.Now().Unix() - ss.LastPull)
		}
		lgInfo.DownStatus = ss.Down.String()
		lgInfo.UpStatus = ss.Up.String()
	}
	return lgInfo
}

type recordInfo struct {
	Version int
	Size    int64
	Time    int64
}

func getRecord(net net.NetBoostrapper, thrd thread.Info, rid cid.Cid, downloadRemote bool) (net3.Record, *recordInfo, error) {
	rinfo := recordInfo{}
	if thrd.ID == thread.Undef {
		return nil, nil, fmt.Errorf("undef id")
	}

	hasBlock, err := net.GetIpfs().HasBlock(rid)
	if err != nil {
		return nil, nil, err
	}
	if !hasBlock && !downloadRemote {
		return nil, nil, fmt.Errorf("don't have record locally")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
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

	s, _ := node.Size()
	rinfo.Size = int64(s)

	m := new(core.SignedPbPayload)
	err = cbornode.DecodeInto(node.RawData(), m)
	if err != nil {
		jp := jsonpatcher.New()
		_, err2 := jp.EventsFromBytes(node.RawData())
		if err2 == nil {
			rinfo.Version = -1
			return rec, &rinfo, nil
		} else {
			return nil, nil, fmt.Errorf("cbor decode error: %w", err)

		}
	}

	err = m.Verify()
	if err != nil {
		return nil, nil, err
	}
	rinfo.Version = int(m.Ver)
	if m.Ver > 0 {
		sbe := core.SmartblockRecordEnvelope{SmartblockRecord: core.SmartblockRecord{ID: rid.String(), PrevID: rec.PrevID().String(), Payload: m.Data}}
		ch, _ := change.NewChangeFromRecord(sbe)
		if ch != nil {
			rinfo.Time = ch.Timestamp
		}
	} else {
		var snapshot = storage.SmartBlockSnapshot{}
		err = m.Unmarshal(&snapshot)
		if err == nil {
			rinfo.Time = snapshot.ClientTime
		}
	}

	return rec, &rinfo, nil
}
