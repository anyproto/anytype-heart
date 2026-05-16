package chatobject

import (
	"testing"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
)

// scenarioCatalog is the table of multi-device read-counter scenarios. Each
// entry's checkpoint WantUnread is authored from the STRICT DAG-ANCESTOR model
// (the current contract): reading head H clears exactly ancestors-of-H;
// concurrent changes stay unread; own-author (testCreator) messages never count
// for that account. TestChatReadCounterScenarios records intended vs actual.
var scenarioCatalog = []scenario{
	{
		Name:    "S-baseline-linear",
		Devices: []string{"A"},
		DAG: []scenarioChange{
			{ID: "G", Kind: kindSystem},
			{ID: "m1", Prev: []string{"G"}, Author: testCreator, Kind: kindMessage},
			{ID: "m2", Prev: []string{"m1"}, Author: "bob", Kind: kindMessage},
			{ID: "m3", Prev: []string{"m2"}, Author: "bob", Kind: kindMessage},
		},
		Steps: []step{
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 2}}},
			{Kind: stepReadUpTo, Device: "A", UpToMsgID: "m3", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 0}}},
		},
		Intent: "Linear single-device: reading the head clears all peer messages.",
	},
	{
		Name:    "S-concurrent-merge-divergence",
		Devices: []string{"A", "B"},
		DAG: []scenarioChange{
			{ID: "G", Kind: kindSystem},
			{ID: "m1", Prev: []string{"G"}, Author: "alice", Kind: kindMessage},
			{ID: "x1", Prev: []string{"G"}, Author: "bob", Kind: kindMessage},
			{ID: "mm", Prev: []string{"m1", "x1"}, Author: "alice", Kind: kindSystem},
		},
		Steps: []step{
			{Kind: stepReadUpTo, Device: "B", UpToMsgID: "x1", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepSync, SyncWith: []string{"A", "B"}, Counter: chatmodel.CounterTypeMessage},
			// Intent = strict DAG-ancestor: ancestors({x1})={G,x1}; m1 stays unread => 1.
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 1}}},
		},
		Intent: "DAG-ancestor model: seeing x1 does not clear concurrent m1. Crux differential case vs OrderId-prefix redesign.",
	},
	{
		Name:    "S-cat1-linear-mention",
		Devices: []string{"A"},
		DAG: []scenarioChange{
			{ID: "G", Kind: kindSystem},
			{ID: "m1", Prev: []string{"G"}, Author: testCreator, Kind: kindMessage},
			{ID: "p1", Prev: []string{"m1"}, Author: "bob", Kind: kindMessage},
			{ID: "mn", Prev: []string{"p1"}, Author: "bob", Kind: kindMention, Mention: testCreator},
			{ID: "p2", Prev: []string{"mn"}, Author: "bob", Kind: kindMessage},
		},
		Steps: []step{
			{Kind: stepCheckpoint, Expect: []checkpointExpect{
				{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 3},
				{Device: "A", Counter: chatmodel.CounterTypeMention, WantUnread: 1},
			}},
			{Kind: stepReadUpTo, Device: "A", UpToMsgID: "mn", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{
				{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 1},
				{Device: "A", Counter: chatmodel.CounterTypeMention, WantUnread: 1},
			}},
			{Kind: stepReadUpTo, Device: "A", UpToMsgID: "p2", Counter: chatmodel.CounterTypeMention},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{
				{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 1},
				{Device: "A", Counter: chatmodel.CounterTypeMention, WantUnread: 0},
			}},
			{Kind: stepReadUpTo, Device: "A", UpToMsgID: "p2", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{
				{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 0},
				{Device: "A", Counter: chatmodel.CounterTypeMention, WantUnread: 0},
			}},
		},
		Intent: "Linear single-device with a mention: per-counter engines are independent; reading the message head leaves the mention counter set until a mention-counter read covers the mention row.",
	},
	{
		Name:    "S-cat2-twodev-sequential",
		Devices: []string{"A", "B"},
		DAG: []scenarioChange{
			{ID: "G", Kind: kindSystem},
			{ID: "s1", Prev: []string{"G"}, Author: testCreator, Kind: kindMessage},
			{ID: "s2", Prev: []string{"s1"}, Author: "carol", Kind: kindMessage},
			{ID: "s3", Prev: []string{"s2"}, Author: "carol", Kind: kindMessage},
			{ID: "s4", Prev: []string{"s3"}, Author: "carol", Kind: kindMessage},
		},
		Steps: []step{
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 3}}},
			{Kind: stepReadUpTo, Device: "A", UpToMsgID: "s2", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 2}}},
			{Kind: stepReadUpTo, Device: "B", UpToMsgID: "s4", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "B", Counter: chatmodel.CounterTypeMessage, WantUnread: 0}}},
		},
		Intent: "Two devices, no concurrency: sequential reads on a linear chat accumulate monotonically to zero unread.",
	},
	{
		Name:    "S-cat3-branch-no-merge",
		Devices: []string{"A", "B"},
		DAG: []scenarioChange{
			{ID: "G", Kind: kindSystem},
			{ID: "r1", Prev: []string{"G"}, Author: "dave", Kind: kindMessage},
			{ID: "ba", Prev: []string{"r1"}, Author: "dave", Kind: kindMessage},
			{ID: "bb", Prev: []string{"r1"}, Author: "erin", Kind: kindMessage},
		},
		Steps: []step{
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 3}}},
			{Kind: stepReadUpTo, Device: "A", UpToMsgID: "ba", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 1}}},
			{Kind: stepReadUpTo, Device: "B", UpToMsgID: "bb", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 0}}},
		},
		Intent: "Concurrent sends with no merge (two live heads): reading one head leaves the concurrent head unread until it too is read.",
	},
	{
		Name:    "S-cat4-concurrent-merge",
		Devices: []string{"A", "B"},
		DAG: []scenarioChange{
			{ID: "G", Kind: kindSystem},
			{ID: "c1", Prev: []string{"G"}, Author: "frank", Kind: kindMessage},
			{ID: "c2", Prev: []string{"G"}, Author: "grace", Kind: kindMessage},
			{ID: "mg", Prev: []string{"c1", "c2"}, Author: "frank", Kind: kindSystem},
			{ID: "c3", Prev: []string{"mg"}, Author: "grace", Kind: kindMessage},
		},
		Steps: []step{
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 3}}},
			{Kind: stepReadUpTo, Device: "A", UpToMsgID: "c1", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 2}}},
			{Kind: stepReadUpTo, Device: "B", UpToMsgID: "mg", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "B", Counter: chatmodel.CounterTypeMessage, WantUnread: 1}}},
			{Kind: stepReadUpTo, Device: "B", UpToMsgID: "c3", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "B", Counter: chatmodel.CounterTypeMessage, WantUnread: 0}}},
		},
		Intent: "Concurrent sends joined by a merge change: seeing the merge clears both concurrent parents (their DAG ancestors); a post-merge message stays unread until read.",
	},
	{
		Name:    "S-cat5-offline-read-then-sync",
		Devices: []string{"A", "B"},
		DAG: []scenarioChange{
			{ID: "G", Kind: kindSystem},
			{ID: "o1", Prev: []string{"G"}, Author: "heidi", Kind: kindMessage},
			{ID: "o2", Prev: []string{"o1"}, Author: "heidi", Kind: kindMessage},
			{ID: "o3", Prev: []string{"o2"}, Author: "heidi", Kind: kindMessage},
		},
		Steps: []step{
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 3}}},
			{Kind: stepReadUpTo, Device: "B", UpToMsgID: "o2", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "B", Counter: chatmodel.CounterTypeMessage, WantUnread: 1}}},
			{Kind: stepSync, SyncWith: []string{"A", "B"}, Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 1}}},
		},
		Intent: "Offline read then sync: the seenHeads union is exactly the offline device's recorded value; sync clears only its ancestors, leaving later messages unread.",
	},
	{
		Name:    "S-cat6-cross-device-convergence",
		Devices: []string{"A", "B"},
		DAG: []scenarioChange{
			{ID: "G", Kind: kindSystem},
			{ID: "v1", Prev: []string{"G"}, Author: "ivan", Kind: kindMessage},
			{ID: "v2", Prev: []string{"v1"}, Author: "ivan", Kind: kindMessage},
			{ID: "v3", Prev: []string{"v2"}, Author: "ivan", Kind: kindMessage},
		},
		Steps: []step{
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "B", Counter: chatmodel.CounterTypeMessage, WantUnread: 3}}},
			{Kind: stepReadUpTo, Device: "A", UpToMsgID: "v3", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 0}}},
			{Kind: stepSync, SyncWith: []string{"A", "B"}, Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "B", Counter: chatmodel.CounterTypeMessage, WantUnread: 0}}},
		},
		Intent: "Cross-device convergence: device A reading to the head and syncing propagates the cleared state so device B observes zero unread (global repo state).",
	},
	{
		Name:    "S-cat7-seenhead-not-present",
		Devices: []string{"A", "B"},
		DAG: []scenarioChange{
			{ID: "G", Kind: kindSystem},
			{ID: "k1", Prev: []string{"G"}, Author: "judy", Kind: kindMessage},
			{ID: "k2", Prev: []string{"k1"}, Author: "judy", Kind: kindMessage},
		},
		Steps: []step{
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 2}}},
			{Kind: stepReadUpTo, Device: "B", UpToMsgID: "k1", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepSync, SyncWith: []string{"B"}, Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 1}}},
			{Kind: stepReadUpTo, Device: "A", UpToMsgID: "k2", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 0}}},
		},
		Intent: "A peer's seenHead references a change behind the local head (pending/not-yet-current view): only that change's ancestors clear; later messages stay unread until the head is read.",
	},
	{
		Name:    "S-cat8-read-regression-resync",
		Devices: []string{"A", "B"},
		DAG: []scenarioChange{
			{ID: "G", Kind: kindSystem},
			{ID: "d1", Prev: []string{"G"}, Author: "ken", Kind: kindMessage},
			{ID: "d2", Prev: []string{"d1"}, Author: "ken", Kind: kindMessage},
			{ID: "d3", Prev: []string{"d2"}, Author: "ken", Kind: kindMessage},
		},
		Steps: []step{
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 3}}},
			{Kind: stepReadUpTo, Device: "A", UpToMsgID: "d3", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 0}}},
			{Kind: stepMarkUnread, Device: "A", AfterOrder: "", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 3}}},
			{Kind: stepSync, SyncWith: []string{"A", "B"}, Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 0}}},
		},
		Intent: "Read regression then re-sync: MarkMessagesAsUnread regresses all rows to unread; re-applying the persisted seenHeads union restores the read state.",
	},
	{
		Name:    "S-cat9-mention-concurrency-analogue",
		Devices: []string{"A", "B"},
		DAG: []scenarioChange{
			{ID: "G", Kind: kindSystem},
			{ID: "base", Prev: []string{"G"}, Author: "leo", Kind: kindMessage},
			{ID: "mnA", Prev: []string{"base"}, Author: "leo", Kind: kindMention, Mention: testCreator},
			{ID: "mnB", Prev: []string{"base"}, Author: "mia", Kind: kindMention, Mention: testCreator},
			{ID: "rxA", Prev: []string{"mnA"}, Author: testCreator, Kind: kindReaction, ReactTo: "mnA"},
			{ID: "rxB", Prev: []string{"mnB"}, Author: "mia", Kind: kindReaction, ReactTo: "mnB"},
			{ID: "mj", Prev: []string{"rxA", "rxB"}, Author: "leo", Kind: kindSystem},
		},
		Steps: []step{
			{Kind: stepCheckpoint, Expect: []checkpointExpect{
				{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 3},
				{Device: "A", Counter: chatmodel.CounterTypeMention, WantUnread: 2},
			}},
			{Kind: stepReadUpTo, Device: "A", UpToMsgID: "rxA", Counter: chatmodel.CounterTypeMention},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{
				{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 3},
				{Device: "A", Counter: chatmodel.CounterTypeMention, WantUnread: 1},
			}},
			{Kind: stepReadUpTo, Device: "B", UpToMsgID: "mj", Counter: chatmodel.CounterTypeMention},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{
				{Device: "B", Counter: chatmodel.CounterTypeMessage, WantUnread: 3},
				{Device: "B", Counter: chatmodel.CounterTypeMention, WantUnread: 0},
			}},
		},
		Intent: "Mention-counter analogue of the reactions-under-concurrency case (no reaction counter exists): concurrent peer mentions with own/peer reaction tips in the DAG; reactions are DAG-only and never counted, the merge clears both concurrent mention rows for the mention counter while the message counter is unaffected.",
	},
	{
		Name:    "S-cat10-mention-vs-nonmention",
		Devices: []string{"A"},
		DAG: []scenarioChange{
			{ID: "G", Kind: kindSystem},
			{ID: "own", Prev: []string{"G"}, Author: testCreator, Kind: kindMessage},
			{ID: "txt", Prev: []string{"own"}, Author: "nina", Kind: kindMessage},
			{ID: "men", Prev: []string{"txt"}, Author: "nina", Kind: kindMention, Mention: testCreator},
			{ID: "txt2", Prev: []string{"men"}, Author: "nina", Kind: kindMessage},
		},
		Steps: []step{
			{Kind: stepCheckpoint, Expect: []checkpointExpect{
				{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 3},
				{Device: "A", Counter: chatmodel.CounterTypeMention, WantUnread: 1},
			}},
			{Kind: stepReadUpTo, Device: "A", UpToMsgID: "txt2", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{
				{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 0},
				{Device: "A", Counter: chatmodel.CounterTypeMention, WantUnread: 1},
			}},
			{Kind: stepReadUpTo, Device: "A", UpToMsgID: "txt", Counter: chatmodel.CounterTypeMention},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{
				{Device: "A", Counter: chatmodel.CounterTypeMention, WantUnread: 1},
			}},
			{Kind: stepReadUpTo, Device: "A", UpToMsgID: "txt2", Counter: chatmodel.CounterTypeMention},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{
				{Device: "A", Counter: chatmodel.CounterTypeMention, WantUnread: 0},
			}},
		},
		Intent: "Mention vs non-mention: only mention rows count for the mention counter; a mention-counter read before the mention does not clear it, a read past it does, independent of the message counter.",
	},
	{
		Name:    "S-cat11-cold-restart-midscenario",
		Devices: []string{"A", "B"},
		DAG: []scenarioChange{
			{ID: "G", Kind: kindSystem},
			{ID: "t1", Prev: []string{"G"}, Author: "omar", Kind: kindMessage},
			{ID: "t2", Prev: []string{"t1"}, Author: "omar", Kind: kindMessage},
			{ID: "t3", Prev: []string{"t2"}, Author: "omar", Kind: kindMessage},
			{ID: "t4", Prev: []string{"t3"}, Author: "omar", Kind: kindMessage},
		},
		Steps: []step{
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 4}}},
			{Kind: stepReadUpTo, Device: "A", UpToMsgID: "t2", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 2}}},
			{Kind: stepRestart, Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 2}}},
			{Kind: stepReadUpTo, Device: "B", UpToMsgID: "t4", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepRestart, Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "B", Counter: chatmodel.CounterTypeMessage, WantUnread: 0}}},
		},
		Intent: "Cold restart mid-scenario (snapshot-cache path): rebuilding the engine from the persisted seenHeads union is idempotent without new tail reads, and correctly clears further once a device has advanced to a new tail change.",
	},
	{
		Name:    "S-cat12-dag-ancestor-vs-orderid-prefix",
		Devices: []string{"A", "B"},
		DAG: []scenarioChange{
			{ID: "G", Kind: kindSystem},
			{ID: "e1", Prev: []string{"G"}, Author: "pat", Kind: kindMessage},
			{ID: "early", Prev: []string{"e1"}, Author: "quinn", Kind: kindMessage},
			{ID: "seen", Prev: []string{"e1"}, Author: "rita", Kind: kindMessage},
			{ID: "join", Prev: []string{"early", "seen"}, Author: "pat", Kind: kindSystem},
			{ID: "late", Prev: []string{"join"}, Author: "quinn", Kind: kindMessage},
		},
		Steps: []step{
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "B", Counter: chatmodel.CounterTypeMessage, WantUnread: 4}}},
			{Kind: stepReadUpTo, Device: "B", UpToMsgID: "seen", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepSync, SyncWith: []string{"A", "B"}, Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "B", Counter: chatmodel.CounterTypeMessage, WantUnread: 2}}},
			{Kind: stepReadUpTo, Device: "B", UpToMsgID: "join", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepSync, SyncWith: []string{"A", "B"}, Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "B", Counter: chatmodel.CounterTypeMessage, WantUnread: 1}}},
		},
		Intent: "Explicit DAG-ancestor vs OrderId-prefix divergence: a concurrent message that sorts before the seen head stays unread under the strict DAG-ancestor contract (the redesign decision case); only seeing the merge clears it.",
	},
	{
		Name:    "S-cat13-three-devices-interleaved",
		Devices: []string{"A", "B", "C"},
		DAG: []scenarioChange{
			{ID: "G", Kind: kindSystem},
			{ID: "i1", Prev: []string{"G"}, Author: "sam", Kind: kindMessage},
			{ID: "i2", Prev: []string{"i1"}, Author: "sam", Kind: kindMessage},
			{ID: "fa", Prev: []string{"i2"}, Author: "sam", Kind: kindMessage},
			{ID: "fb", Prev: []string{"i2"}, Author: "tom", Kind: kindMessage},
			{ID: "im", Prev: []string{"fa", "fb"}, Author: "sam", Kind: kindSystem},
			{ID: "i3", Prev: []string{"im"}, Author: "tom", Kind: kindMessage},
		},
		Steps: []step{
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 5}}},
			{Kind: stepReadUpTo, Device: "A", UpToMsgID: "i1", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepReadUpTo, Device: "B", UpToMsgID: "fa", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepReadUpTo, Device: "C", UpToMsgID: "fb", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "B", Counter: chatmodel.CounterTypeMessage, WantUnread: 1}}},
			{Kind: stepSync, SyncWith: []string{"A", "B", "C"}, Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "C", Counter: chatmodel.CounterTypeMessage, WantUnread: 1}}},
			{Kind: stepReadUpTo, Device: "C", UpToMsgID: "i3", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "C", Counter: chatmodel.CounterTypeMessage, WantUnread: 0}}},
		},
		Intent: "Three devices with interleaved reads across a concurrent branch+merge: the cleared set is the cumulative union of every device's read-ancestor set; the post-merge message clears only when some device reads the head.",
	},
	{
		Name:    "S-cat14-long-divergent-branches-merge",
		Devices: []string{"A", "B"},
		DAG: []scenarioChange{
			{ID: "G", Kind: kindSystem},
			{ID: "p0", Prev: []string{"G"}, Author: "uma", Kind: kindMessage},
			{ID: "L1", Prev: []string{"p0"}, Author: "uma", Kind: kindMessage},
			{ID: "L2", Prev: []string{"L1"}, Author: "uma", Kind: kindMessage},
			{ID: "L3", Prev: []string{"L2"}, Author: "uma", Kind: kindMessage},
			{ID: "R1", Prev: []string{"p0"}, Author: "vic", Kind: kindMessage},
			{ID: "R2", Prev: []string{"R1"}, Author: "vic", Kind: kindMessage},
			{ID: "R3", Prev: []string{"R2"}, Author: "vic", Kind: kindMessage},
			{ID: "MG", Prev: []string{"L3", "R3"}, Author: "uma", Kind: kindSystem},
			{ID: "tail", Prev: []string{"MG"}, Author: "vic", Kind: kindMessage},
		},
		Steps: []step{
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 8}}},
			{Kind: stepReadUpTo, Device: "A", UpToMsgID: "L3", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 4}}},
			{Kind: stepReadUpTo, Device: "B", UpToMsgID: "R3", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "B", Counter: chatmodel.CounterTypeMessage, WantUnread: 1}}},
			{Kind: stepReadUpTo, Device: "A", UpToMsgID: "MG", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 1}}},
			{Kind: stepReadUpTo, Device: "B", UpToMsgID: "tail", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "B", Counter: chatmodel.CounterTypeMessage, WantUnread: 0}}},
		},
		Intent: "Two long divergent branches then merge: each branch clears only when its own tip is read; the merge clears nothing extra once both tips are seen; the post-merge tail clears last.",
	},
	{
		Name:    "S-cat15-own-message-exclusion-edges",
		Devices: []string{"A", "B"},
		DAG: []scenarioChange{
			{ID: "G", Kind: kindSystem},
			{ID: "own1", Prev: []string{"G"}, Author: testCreator, Kind: kindMessage},
			{ID: "peer1", Prev: []string{"own1"}, Author: "wade", Kind: kindMessage},
			{ID: "ownM", Prev: []string{"own1"}, Author: testCreator, Kind: kindMention, Mention: "wade"},
			{ID: "mrg", Prev: []string{"peer1", "ownM"}, Author: testCreator, Kind: kindSystem},
			{ID: "peerM", Prev: []string{"mrg"}, Author: "wade", Kind: kindMention, Mention: testCreator},
			{ID: "own2", Prev: []string{"peerM"}, Author: testCreator, Kind: kindMessage},
		},
		Steps: []step{
			{Kind: stepCheckpoint, Expect: []checkpointExpect{
				{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 2},
				{Device: "A", Counter: chatmodel.CounterTypeMention, WantUnread: 1},
			}},
			{Kind: stepReadUpTo, Device: "A", UpToMsgID: "mrg", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{
				{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 1},
				{Device: "A", Counter: chatmodel.CounterTypeMention, WantUnread: 1},
			}},
			{Kind: stepReadUpTo, Device: "B", UpToMsgID: "own2", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{
				{Device: "B", Counter: chatmodel.CounterTypeMessage, WantUnread: 0},
				{Device: "B", Counter: chatmodel.CounterTypeMention, WantUnread: 1},
			}},
			{Kind: stepReadUpTo, Device: "B", UpToMsgID: "own2", Counter: chatmodel.CounterTypeMention},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{
				{Device: "B", Counter: chatmodel.CounterTypeMention, WantUnread: 0},
			}},
		},
		Intent: "Own-message exclusion edge cases: own messages and own mentions (even concurrent ones or the head) never count for either counter; only peer rows are counted, and reading an own message as the head still clears peer rows by DAG ancestry.",
	},
	{
		Name:    "S-cat16-notfound-seenhead",
		Devices: []string{"A", "B"},
		DAG: []scenarioChange{
			{ID: "G", Kind: kindSystem},
			{ID: "n1", Prev: []string{"G"}, Author: "al", Kind: kindMessage},
			{ID: "n2", Prev: []string{"n1"}, Author: "al", Kind: kindMessage},
		},
		Steps: []step{
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 2}}},
			// "ghost" is not in the DAG -> DiffManager.notFound; clears nothing
			{Kind: stepReadUpTo, Device: "B", UpToMsgID: "ghost", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "B", Counter: chatmodel.CounterTypeMessage, WantUnread: 2}}},
			// a valid head in the same device value still clears its ancestors
			{Kind: stepReadUpTo, Device: "B", UpToMsgID: "n2", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "B", Counter: chatmodel.CounterTypeMessage, WantUnread: 0}}},
			// sync the mixed value {ghost,n2}: notFound ignored, n2 ancestors still cleared
			{Kind: stepSync, SyncWith: []string{"A", "B"}, Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 0}}},
		},
		Intent: "A recorded seenHead referencing a change absent from the local tree resolves to not-found and clears nothing; a valid head in the same device value still clears its DAG ancestors (notFound path, spec §5 cat-7).",
	},
	{
		Name:    "S-cat17-partial-markunread",
		Devices: []string{"A"},
		DAG: []scenarioChange{
			{ID: "G", Kind: kindSystem},
			{ID: "q1", Prev: []string{"G"}, Author: "be", Kind: kindMessage},
			{ID: "q2", Prev: []string{"q1"}, Author: "be", Kind: kindMessage},
			{ID: "q3", Prev: []string{"q2"}, Author: "be", Kind: kindMessage},
			{ID: "q4", Prev: []string{"q3"}, Author: "be", Kind: kindMessage},
		},
		Steps: []step{
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 4}}},
			{Kind: stepReadUpTo, Device: "A", UpToMsgID: "q4", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 0}}},
			// partial regression strictly AFTER q2 -> q3,q4 unread again; q1,q2 stay read
			{Kind: stepMarkUnread, Device: "A", AfterOrder: "q2", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 2}}},
			// a fresh engine (sync) recomputes from the persisted value and restores reads
			{Kind: stepSync, SyncWith: []string{"A"}, Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 0}}},
		},
		Intent: "Partial MarkMessagesAsUnread regresses only rows strictly after the given change; restoring requires a rebuilt engine (sync) because the live engine's advanced watermark makes a redundant read a no-op.",
	},
	{
		Name:    "S-cat18-multihead-value",
		Devices: []string{"A", "B"},
		DAG: []scenarioChange{
			{ID: "G", Kind: kindSystem},
			{ID: "p0", Prev: []string{"G"}, Author: "ca", Kind: kindMessage},
			{ID: "h1", Prev: []string{"p0"}, Author: "ca", Kind: kindMessage},
			{ID: "h2", Prev: []string{"p0"}, Author: "da", Kind: kindMessage},
			{ID: "w1", Prev: []string{"h1"}, Author: "ca", Kind: kindMessage},
			{ID: "w2", Prev: []string{"h2"}, Author: "da", Kind: kindMessage},
		},
		Steps: []step{
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "B", Counter: chatmodel.CounterTypeMessage, WantUnread: 5}}},
			// B records BOTH concurrent tips into one device value
			{Kind: stepReadUpTo, Device: "B", UpToMsgID: "w1", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepReadUpTo, Device: "B", UpToMsgID: "w2", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "B", Counter: chatmodel.CounterTypeMessage, WantUnread: 0}}},
			{Kind: stepMarkUnread, Device: "A", AfterOrder: "", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 5}}},
			// the multi-head value {w1,w2} alone clears the union of both branches' ancestors
			{Kind: stepSync, SyncWith: []string{"B"}, Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "B", Counter: chatmodel.CounterTypeMessage, WantUnread: 0}}},
		},
		Intent: "A single device's seenHeads value containing two concurrent heads (real CRDT shape) clears the DAG ancestors of both heads in one Remove; after a full regression, syncing that multi-head value alone restores all reads.",
	},
	{
		Name:    "S-cat19-msg-mention-concurrent",
		Devices: []string{"A", "B"},
		DAG: []scenarioChange{
			{ID: "G", Kind: kindSystem},
			{ID: "base", Prev: []string{"G"}, Author: "ed", Kind: kindMessage},
			{ID: "pm", Prev: []string{"base"}, Author: "ed", Kind: kindMessage},
			{ID: "cm", Prev: []string{"base"}, Author: "fi", Kind: kindMention, Mention: testCreator},
			{ID: "mg", Prev: []string{"pm", "cm"}, Author: "ed", Kind: kindSystem},
			{ID: "tailMsg", Prev: []string{"mg"}, Author: "ed", Kind: kindMessage},
		},
		Steps: []step{
			{Kind: stepCheckpoint, Expect: []checkpointExpect{
				{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 4},
				{Device: "A", Counter: chatmodel.CounterTypeMention, WantUnread: 1},
			}},
			{Kind: stepReadUpTo, Device: "A", UpToMsgID: "pm", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{
				{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 2},
				{Device: "A", Counter: chatmodel.CounterTypeMention, WantUnread: 1},
			}},
			{Kind: stepReadUpTo, Device: "B", UpToMsgID: "mg", Counter: chatmodel.CounterTypeMention},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{
				{Device: "B", Counter: chatmodel.CounterTypeMessage, WantUnread: 2},
				{Device: "B", Counter: chatmodel.CounterTypeMention, WantUnread: 0},
			}},
			{Kind: stepReadUpTo, Device: "B", UpToMsgID: "tailMsg", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{
				{Device: "B", Counter: chatmodel.CounterTypeMessage, WantUnread: 0},
				{Device: "B", Counter: chatmodel.CounterTypeMention, WantUnread: 0},
			}},
		},
		Intent: "Per-counter independence under concurrency: a message-counter read of one concurrent branch does not touch a concurrent peer mention's mention counter; only a mention-counter read covering it clears it; the message head later clears the mention row for the message counter only.",
	},
	{
		Name:    "S-cat20-threeway-concurrent-merge",
		Devices: []string{"A", "B"},
		DAG: []scenarioChange{
			{ID: "G", Kind: kindSystem},
			{ID: "r", Prev: []string{"G"}, Author: "ge", Kind: kindMessage},
			{ID: "s1", Prev: []string{"r"}, Author: "ha", Kind: kindMessage},
			{ID: "s2", Prev: []string{"r"}, Author: "ia", Kind: kindMessage},
			{ID: "s3", Prev: []string{"r"}, Author: "ja", Kind: kindMessage},
			{ID: "m3", Prev: []string{"s1", "s2", "s3"}, Author: "ge", Kind: kindSystem},
			{ID: "post", Prev: []string{"m3"}, Author: "ge", Kind: kindMessage},
		},
		Steps: []step{
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 5}}},
			{Kind: stepReadUpTo, Device: "A", UpToMsgID: "s1", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 3}}},
			{Kind: stepReadUpTo, Device: "A", UpToMsgID: "s2", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 2}}},
			{Kind: stepReadUpTo, Device: "B", UpToMsgID: "m3", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "B", Counter: chatmodel.CounterTypeMessage, WantUnread: 1}}},
			{Kind: stepReadUpTo, Device: "B", UpToMsgID: "post", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "B", Counter: chatmodel.CounterTypeMessage, WantUnread: 0}}},
		},
		Intent: "Three concurrent sibling branches off a common parent joined by one merge: each sibling clears only when itself or the merge is seen; the ancestor set is independent of 3-way sibling hash-tiebreak ordering (determinism premise stress).",
	},
	{
		Name:    "S-cat21-mention-multidevice-restart",
		Devices: []string{"A", "B"},
		DAG: []scenarioChange{
			{ID: "G", Kind: kindSystem},
			{ID: "q0", Prev: []string{"G"}, Author: "ka", Kind: kindMessage},
			{ID: "mnL", Prev: []string{"q0"}, Author: "ka", Kind: kindMention, Mention: testCreator},
			{ID: "mnR", Prev: []string{"q0"}, Author: "la", Kind: kindMention, Mention: testCreator},
			{ID: "mj", Prev: []string{"mnL", "mnR"}, Author: "ka", Kind: kindSystem},
			{ID: "mnT", Prev: []string{"mj"}, Author: "ka", Kind: kindMention, Mention: testCreator},
		},
		Steps: []step{
			{Kind: stepCheckpoint, Expect: []checkpointExpect{
				{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 4},
				{Device: "A", Counter: chatmodel.CounterTypeMention, WantUnread: 3},
			}},
			{Kind: stepReadUpTo, Device: "A", UpToMsgID: "mnL", Counter: chatmodel.CounterTypeMention},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{
				{Device: "A", Counter: chatmodel.CounterTypeMention, WantUnread: 2},
				{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 4},
			}},
			{Kind: stepReadUpTo, Device: "B", UpToMsgID: "mnR", Counter: chatmodel.CounterTypeMention},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "B", Counter: chatmodel.CounterTypeMention, WantUnread: 1}}},
			{Kind: stepSync, SyncWith: []string{"A", "B"}, Counter: chatmodel.CounterTypeMention},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMention, WantUnread: 1}}},
			{Kind: stepReadUpTo, Device: "A", UpToMsgID: "mnT", Counter: chatmodel.CounterTypeMention},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMention, WantUnread: 0}}},
			{Kind: stepRestart, Counter: chatmodel.CounterTypeMention},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{
				{Device: "B", Counter: chatmodel.CounterTypeMention, WantUnread: 0},
				{Device: "B", Counter: chatmodel.CounterTypeMessage, WantUnread: 4},
			}},
		},
		Intent: "Mention counter across two devices: concurrent peer mentions cleared per-device then unioned via sync; a cold restart rebuilds the mention engine from the persisted union and preserves the cleared state, with the message counter untouched throughout.",
	},
	{
		Name:    "S-cat22-sync-order-commutative",
		Devices: []string{"A", "B"},
		DAG: []scenarioChange{
			{ID: "G", Kind: kindSystem},
			{ID: "c0", Prev: []string{"G"}, Author: "ma", Kind: kindMessage},
			{ID: "bA", Prev: []string{"c0"}, Author: "ma", Kind: kindMessage},
			{ID: "bB", Prev: []string{"c0"}, Author: "na", Kind: kindMessage},
			{ID: "aA", Prev: []string{"bA"}, Author: "ma", Kind: kindMessage},
			{ID: "aB", Prev: []string{"bB"}, Author: "na", Kind: kindMessage},
		},
		Steps: []step{
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 5}}},
			{Kind: stepReadUpTo, Device: "A", UpToMsgID: "aA", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepReadUpTo, Device: "B", UpToMsgID: "aB", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepMarkUnread, Device: "A", AfterOrder: "", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 5}}},
			// union applied in [B,A] order
			{Kind: stepSync, SyncWith: []string{"B", "A"}, Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 0}}},
			{Kind: stepMarkUnread, Device: "B", AfterOrder: "", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "B", Counter: chatmodel.CounterTypeMessage, WantUnread: 5}}},
			// same union applied in [A,B] order -> identical result
			{Kind: stepSync, SyncWith: []string{"A", "B"}, Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "B", Counter: chatmodel.CounterTypeMessage, WantUnread: 0}}},
		},
		Intent: "seenHeads union is order-independent: syncing devices in [B,A] vs [A,B] order yields identical cleared state (commutative union, validating the multi-device merge simulation, spec §8).",
	},
	{
		Name:    "S-cat23-regression-then-restart",
		Devices: []string{"A", "B"},
		DAG: []scenarioChange{
			{ID: "G", Kind: kindSystem},
			{ID: "d1", Prev: []string{"G"}, Author: "ow", Kind: kindMessage},
			{ID: "d2", Prev: []string{"d1"}, Author: "ow", Kind: kindMessage},
			{ID: "d3", Prev: []string{"d2"}, Author: "ow", Kind: kindMessage},
		},
		Steps: []step{
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 3}}},
			{Kind: stepReadUpTo, Device: "A", UpToMsgID: "d3", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 0}}},
			{Kind: stepMarkUnread, Device: "A", AfterOrder: "", Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 3}}},
			// cold restart rebuilds the engine from the persisted seenHeads union and re-applies markRead
			{Kind: stepRestart, Counter: chatmodel.CounterTypeMessage},
			{Kind: stepCheckpoint, Expect: []checkpointExpect{{Device: "A", Counter: chatmodel.CounterTypeMessage, WantUnread: 0}}},
		},
		Intent: "Read regression followed by a cold restart: MarkMessagesAsUnread flips repo rows unread; rebuilding the engine from the persisted seenHeads union (cold start) re-applies markRead and restores the cleared state (snapshot-cache × regression interaction).",
	},
}

func TestChatReadCounterScenarios(t *testing.T) {
	runCatalog(t, scenarioCatalog)
}
