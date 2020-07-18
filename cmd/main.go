package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-library/core/config"
	"github.com/anytypeio/go-anytype-library/net"
	"github.com/ipfs/go-cid"
	format "github.com/ipfs/go-ipld-format"
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/textileio/go-threads/cbor"
	db2 "github.com/textileio/go-threads/core/db"
	net2 "github.com/textileio/go-threads/core/net"
	"github.com/textileio/go-threads/core/thread"
	"github.com/textileio/go-threads/db"
	"github.com/textileio/go-threads/util"
)

type threadInfo struct {
	ID    db2.InstanceID `json:"_id"`
	Key   string
	Addrs []string
}

type threadRecord struct {
	net2.Record
	threadID thread.ID
	logID    peer.ID
}

func (t threadRecord) Value() net2.Record {
	return t.Record
}

func (t threadRecord) ThreadID() thread.ID {
	return t.threadID
}

func (t threadRecord) LogID() peer.ID {
	return t.logID
}

func main() {
	repo := flag.String("repo", "", "local repo path")
	account := flag.String("account", "", "account ID")

	threads := flag.Bool("threads", false, "List all threads")
	threadId := flag.String("thread", "", "Traverse the thread")
	logId := flag.String("log", "", "Traverse the thread's log")
	handledb := flag.String("handledb", "", "handle all records for the thread's db")
	addMissingReplicator := flag.Bool("addreplicators", false, "add missing cafe replicators")

	flag.Parse()

	if repo == nil {
		log.Fatal("you should provide -repo")
	}

	if account == nil {
		log.Fatal("you should provide -account")
	}

	lib, err := core.New(*repo, *account)
	if err != nil {
		log.Fatal(err.Error())
	}
	err = lib.Start()
	if err != nil {
		log.Fatal(err.Error())
	}

	err = lib.InitPredefinedBlocks(false)
	if err != nil {
		log.Fatal(err.Error())
	}

	if threads != nil && *threads {
		listThreads(lib.(*core.Anytype))
		return
	}

	if threadId != nil && *threadId != "" {
		if logId == nil || *logId == "" {
			log.Fatal("you need to specify -log")
		}
		traverseThread(lib.(*core.Anytype), *threadId, *logId)
	}

	if handledb != nil && *handledb != "" {
		catchAllLogs(lib.(*core.Anytype), *handledb)
	}

	if addMissingReplicator != nil && *addMissingReplicator {
		addMissingReplicators(*repo, lib.(*core.Anytype))
	}
}

func getRecord(net net.NetBoostrapper, thrd thread.Info, rid cid.Cid) (net2.Record, *cbor.Event, format.Node, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	if thrd.ID == thread.Undef {
		return nil, nil, nil, fmt.Errorf("undef id")
	}

	rec, err := net.GetRecord(ctx, thrd.ID, rid)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load record: %s", err.Error())
	}

	event, err := cbor.EventFromRecord(ctx, net, rec)
	if err != nil {
		return rec, nil, nil, fmt.Errorf("failed to load event: %s", err.Error())
	}

	node, err := event.GetBody(context.TODO(), net, thrd.Key.Read())
	if err != nil {
		return rec, event, nil, fmt.Errorf("failed to get record body: %w", err)
	}

	return rec, event, node, nil
}

func printAllRecords(net net.NetBoostrapper, thrd thread.Info, li thread.LogInfo) {
	rid := li.Head
	total := 0
	defer func() {
		fmt.Printf("total %d records\n", total)
	}()

	for {
		total++
		rec, event, node, err := getRecord(net, thrd, rid)
		if rec != nil {
			var nodeSize int
			if node != nil {
				stat, _ := node.Stat()
				if stat != nil {
					nodeSize = stat.DataSize
				}
			}
			fmt.Printf("%s: event %v, node %v, size %d\n", rid.String(), event != nil, node != nil, nodeSize)
		} else {
			fmt.Printf("%s: %v\n", rid.String(), err)
		}
		if rec != nil {
			rid = rec.PrevID()
			if !rid.Defined() {
				fmt.Printf("found the first entry\n")
				return
			}
		} else {
			fmt.Printf("can't continue the traverse because failed to load a record\n")
			return
		}
	}
}

func catchAllRecords(tdb *db.DB, net net.NetBoostrapper, thrd thread.Info, li thread.LogInfo) {
	rid := li.Head
	total := 0
	var records []threadRecord
	ownLog := thrd.GetOwnLog()

	defer func() {
		for i := len(records) - 1; i >= 0; i-- {
			err := tdb.HandleNetRecord(records[i], thrd.Key, ownLog.ID, time.Second*5)
			if err != nil {
				fmt.Printf("failed to handle record: %s\n", err.Error())
			}
		}

		fmt.Printf("total %d records\n", total)
	}()

	for {
		if !rid.Defined() {
			fmt.Printf("found the first entry\n")
			return
		}
		total++
		rec, event, node, err := getRecord(net, thrd, rid)
		if rec != nil {
			var nodeSize int
			if node != nil {
				stat, _ := node.Stat()
				if stat != nil {
					nodeSize = stat.DataSize
				}
			}
			fmt.Printf("%s: event %v, node %v, size %d\n", rid.String(), event != nil, node != nil, nodeSize)
		} else {
			fmt.Printf("%s: %v\n", rid.String(), err)
		}
		if rec != nil {
			trec := threadRecord{
				Record:   rec,
				threadID: thrd.ID,
				logID:    li.ID,
			}

			records = append(records, trec)
			rid = rec.PrevID()
		} else {
			fmt.Printf("can't continue the traverse because failed to load a record\n")
			return
		}
	}
}

func traverseThread(lib *core.Anytype, threadId string, logId string) {
	tid, err := thread.Decode(threadId)
	if err != nil {
		log.Fatalf("failed to parse thread id %s: %s", threadId, err.Error())
	}

	lid, err := peer.Decode(logId)
	if err != nil {
		log.Fatalf("failed to parse log id %s: %s", logId, err.Error())
	}

	thrd, err := lib.ThreadsNet().GetThread(context.Background(), tid)
	if err != nil {
		log.Fatalf("failed to get thread info: %s\n", err.Error())
	}

	var logFound thread.LogInfo
	for _, logInfo := range thrd.Logs {
		if logInfo.ID.String() == lid.String() {
			logFound = logInfo
			break
		}
	}

	if logFound.ID.Size() == 0 {
		log.Fatalf("failed to find log %s in thread %s\n", logId, threadId)
	}

	printAllRecords(lib.ThreadsNet(), thrd, logFound)
}

func catchAllLogs(lib *core.Anytype, threadId string) {
	tid, err := thread.Decode(threadId)
	if err != nil {
		log.Fatalf("failed to parse thread id %s: %s", threadId, err.Error())
	}

	thrd, err := lib.ThreadsNet().GetThread(context.Background(), tid)
	if err != nil {
		log.Fatalf("failed to get thread info: %s\n", err.Error())
	}

	for _, logInfo := range thrd.Logs {
		fmt.Printf("traversing %s log from head %s\n", logInfo.ID, logInfo.Head)
		catchAllRecords(lib.ThreadsDB(), lib.ThreadsNet(), thrd, logInfo)
	}
}

func listThreads(lib *core.Anytype) {
	pblocks := lib.PredefinedBlocks()
	fmt.Printf("Account: %s\n", pblocks.Account)
	fmt.Printf("Home: %s\n", pblocks.Home)
	fmt.Printf("Profile: %s\n", pblocks.Profile)
	fmt.Printf("Archive: %s\n", pblocks.Archive)
	fmt.Printf("Set pages: %s\n", pblocks.SetPages)
	fmt.Println("")

	threadsCollection := lib.ThreadsCollection()
	instancesBytes, err := threadsCollection.Find(&db.Query{})
	if err != nil {
		log.Fatal(err.Error())
	}
	fmt.Printf("Threads in the collection: %d\n\n", len(instancesBytes))

	var threadsInCollection = make(map[string]threadInfo)
	for _, instanceBytes := range instancesBytes {
		ti := threadInfo{}
		util.InstanceFromJSON(instanceBytes, &ti)

		tid, err := thread.Decode(ti.ID.String())
		if err != nil {
			log.Printf("failed to parse thread id %s: %s", ti.ID, err.Error())
			continue
		}
		threadsInCollection[tid.String()] = ti
	}

	threadsIds, err := lib.ThreadsNet().Logstore().Threads()
	if err != nil {
		log.Fatal(err.Error())
	}

	var threadsInLogstore = make(map[string]struct{})

	for _, threadId := range threadsIds {
		fmt.Printf("%s\n", threadId.String())
		if v, exists := threadsInCollection[threadId.String()]; exists {
			fmt.Printf("addrs in coll: %v\n", v.Addrs)
		} else {
			fmt.Printf("not exists in col\n")
		}

		thrd, err := lib.ThreadsNet().GetThread(context.Background(), threadId)
		if err != nil {
			fmt.Printf("error getting info: %s\n", err.Error())
		} else {
			fmt.Printf("Addrs: %v\n", thrd.Addrs)
		}
		fmt.Println("Logs:")
		for _, logInfo := range thrd.Logs {
			fmt.Printf("%s: own %v; head %s; addrs: %v\n", logInfo.ID.String(), logInfo.PrivKey != nil, logInfo.Head, logInfo.Addrs)
		}
		threadsInLogstore[threadId.String()] = struct{}{}
		fmt.Printf("\n\n")
	}

	for tid, _ := range threadsInCollection {
		if _, exists := threadsInLogstore[tid]; !exists {
			fmt.Printf("%s found only in collection\n", tid)
		}
	}
}

func addMissingReplicators(repoPath string, lib *core.Anytype) {
	threadsIds, err := lib.ThreadsNet().Logstore().Threads()
	if err != nil {
		log.Fatal(err.Error())
	}

	cfg, err := config.GetConfig(repoPath)
	var threadsInLogstore = make(map[string]struct{})

	cafeAddr, err := ma.NewMultiaddr(cfg.CafeP2PAddr)
	if err != nil {
		log.Fatal(err.Error())
	}

	for _, threadId := range threadsIds {
		thrd, err := lib.ThreadsNet().GetThread(context.Background(), threadId)
		if err != nil {
			fmt.Printf("error getting info: %s\n", err.Error())
		} else {
			fmt.Printf("Addrs: %v\n", thrd.Addrs)
		}

		fmt.Println("Logs:")
		exists := false

		for _, addr := range thrd.Addrs {
			p2paddr, err := addr.ValueForProtocol(ma.P_P2P)
			if err == nil {
				if p2paddr == "12D3KooWKwPC165PptjnzYzGrEs7NSjsF5vvMmxmuqpA2VfaBbLw" {
					exists = true
					break
				}
			}
		}
		if !exists {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
			defer cancel()
			_, err := lib.ThreadsNet().AddReplicator(ctx, thrd.ID, cafeAddr)
			if err != nil {
				fmt.Printf("failed to add replicator for %s: %s\n", thrd.ID, err.Error())
			} else {
				fmt.Printf("%s: added replicator\n", thrd.ID)
			}
		}

		threadsInLogstore[threadId.String()] = struct{}{}
		fmt.Printf("\n\n")
	}
}
