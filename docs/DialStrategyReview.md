# Dial rework — 5-lens adversarial review (consolidated)

Date: 2026-08-03. Five independent Opus reviewers attacked
`docs/DialStrategyRework.md` v1 through distinct lenses: **local-only/P2P**,
**slow networks**, **partial blocking/middleboxes**, **concurrency/lifecycle**,
**rollout/compat**. All findings below were verified against the worktrees
(heart c396cb040, any-sync 9cd8cd75) unless marked PLAUSIBLE. The design doc
has been revised to v2 in response; the "resolution" column names the v2 item
that absorbs each finding.

**Overall verdict:** the architecture direction survives (concurrency over
budgets, evidence outside quic-go, node/local separation, epoch tags), but v1
was not implementable as written. Ten findings were design-changing; two
sequencing orderings in v1 were actively dangerous (R1, R2 below); and the
review produced one new root-cause lead for the original incident (R0).

## R0 — New root-cause lead for the GO-7409 incident

A server config with `quic.listenAddrs` set but `dialTimeoutSec` **unset**
yields `WithTimeout(Background, 0)` on the accept path (`quic.go:190`): the
server completes the QUIC/TLS handshake, then instantly abandons
`AcceptStream` — **without closing the conn** (leak) — while the client's
`HandshakeOutbound` blocks forever on a stream nobody will answer, kept alive
by the 10s keepalive (client `MaxIdleTimeout` = 30s default never fires
because keepalives flow). From the client this is indistinguishable from a
firewall black hole, but no firewall repro reproduces it — consistent with
the July repro failing to survive scrutiny. Stock any-sync-dockercompose
emits `dialTimeoutSec: 10`, so the exposed population is hand-edited /
partially-migrated configs. **Action: get the affected self-hoster's node
`config.yml` quic block; re-run the repro against a node with
`dialTimeoutSec` deleted.**

## Critical findings

| # | lens | finding | resolution in v2 |
|---|---|---|---|
| C1 | concurrency | Fan-out with full secure handshakes produces 2 inbound conns per dial at the remote; `pool.AddPeer` (pool.go:169-183) closes the *existing* and keeps the newest, while the dialer keeps the *first* — the two sides systematically keep opposite conns → both die → permanent flap loop. Old servers are the remote, so no client-side fix alone suffices if handshakes race. | D2 restructured: race **transport establishment only**, run the any-sync handshake on exactly one candidate at a time (no duplicate ever reaches `AddPeer`, works against old servers); `AddPeer` keep-alive-existing lands in PR-A as defense-in-depth for mutual-dial races. |
| C2 | slow-net + concurrency | D1 v1 did **not** retire the 851d38f4 abort-retry — it armed it: `loadAborted` derives from the ocache-level (first-caller) ctx (ocache.go:200-209), which detachment makes "always aborted" whenever the first caller leaves; waiters retry ≤3× → up to 4×30s per peerId, ×3 nodes ≈ 6 min fetch with no status flip. (Only on v0.13.x — `main` has no abort-retry.) | D1 is now an explicit `ocache` detached-load option: load ctx re-rooted **inside ocache**, abort flag computed from the real load ctx, per-entry cancel handles. |
| C3 | blocking | "Handshake succeeds, then flow is killed" scores as dial **success**: BH counter votes Allowed, backoff clears, conn dies at the 30s idle floor, rebuild re-dials into the same trap — steady-state ~5s useful / ~30s dead forever. Nothing in v1 consumed connection-lifetime evidence. | New D9 (connection liveness): early-death events (die < ~60s with idle/keepalive cause) feed BH + backoff as failures; `MaxIdleTimeoutSec` exposed in any-sync config, heart sets ~12s; `GetResponsiblePeers` gates on inbound-activity, not just `IsClosed`. |
| C4 | blocking + slow-net | libp2p's BH parameters don't fit our dial volume: N=100 needs ~11 min of pure failure to engage (9 outcomes/min), stops accruing once a yamux conn is pooled, and v1's reset fired on every `recover()` (incl. foreground resume >15s) — structurally pinned in Probing. Count-based 1-in-N probing ≈ hours to recover. | D4 recalibrated: N≈8/MinSuccesses≈2 with sample aging, keyed (epoch, scheme, provenance); **skip**-when-Blocked with a **time-based** probe (60-120s); reset only on genuine epoch change. |
| C5 | blocking + local-only (convergent) | Scoping BH/backoff by IP class ("public only") excludes self-hosted RFC1918 **nodes** — the exact incident population — while conflating them with LAN neighbours. §5.1 requires node-vs-local, which is *provenance*, not IP class. | D4/D5 keyed by provenance: peerId ∈ `nodeConf.PeerAddresses` ⇒ node; `peerAddrs` (mDNS/push) ⇒ excluded. IP class only for stagger timing. |
| C6 | local-only | D8 v1's absolute peer-find deadline + the spaceloader's once-created ctx (spaceloader.go:70, reused by every retry) = after 30s every retry fails in µs; a LAN-only space becomes **permanently unloadable** (v1 turned "hangs but eventually loads" into "never loads"). Same bug already shipped: `fileobject/service.go:176-179` stamps a 1-min deadline into a long-lived ctx under `IsLocalOnlyMode`. Also bypassable via ocache single-flight when `SyncAllSpaceHeads` wins the load race. | D8 rewritten: duration-valued key resolved at wait time, stamped per attempt; `ErrPeerFindDeadlineExceeded` mapped so `spaceservice`'s loop keeps polling; sequenced **after** the any-sync bump (see R1); fileobject bug fixed now. |
| C7 | local-only | "Re-discovery re-adds within seconds" is false: zeroconf suppresses re-announcement of known entries for the full TTL (3600s, no re-broadcast; sentEntries, client.go:301) — any wrongful local-peer eviction severs LAN sync for up to an hour. Invalidated the stated mitigations of D1, D5 and D7 simultaneously. | D7 adds a re-announce path (TTL ↓ ~120s and/or forced browse restart on any eviction); eviction gated on *definitive* failure only; Android (no re-announce control) gets conservative demote-only. |
| C8 | local-only + concurrency | D1 defeats heart's 5s local bound twice: `pool.Pick(n.ctx, …)` (manager.go:394) blocks unbounded on the in-flight load (ocache `Pick` = `waitLoad`), and the socket fan-out continues 25s after the caller's 5s wait expires. | `TryPick` exported on `pool.Pool` (not just internal) and used at the manager fast path; local dials get a shorter detached window + in-flight cap ≤3. |
| C9 | rollout | §5 v1's branch labels were inverted: any-sync `main` **is** the 0.12 line (tip = v0.12.18); v0.13.x is a separate branch and a protocol-epoch migration (ProtoVersion 12→13, drops compat v9, files-v2, SpaceExchangeV2, DiffType_V2 removal). "PR-A on main, PR-C on main (0.13.x)" would have left PR-C's base without the zero-guard — reproducing the exact landmine the design warns about. | §5 rewritten: all dial PRs land on `main` (dial files verified byte-identical across lines), merge forward to v0.13.x; heart bumps to **v0.12.20**, decoupled from the protocol migration. |
| C10 | slow-net | Lost-race dials recorded as failures create self-reinforcing scheme lock-in (quic loses one coin-flip race on a lossy link → demoted → starts +500ms behind → can never win again). libp2p itself warns about this and mitigates via skip-not-demote. | D2 outcome rules: cancelled losers are **neutral**; only terminal own-error results are recorded; preferred-scheme grace window (~500ms) before accepting a fallback winner. |

## Major findings

| # | lens | finding | resolution in v2 |
|---|---|---|---|
| M1 | blocking + rollout + concurrency (convergent ×3) | `errors.Join` widens `Is/As` from "last error" to "any leaf": one MITM'd/stale addr → 20-min `errObject` blackball of a working peer + `pool.pick` panic; `spaceservice` retry loop inverts to hard failure; `nodeconf`'s bare `switch err` stops matching entirely (compat status degrades). | D2.6: classification from the **set** (unanimity required for `HandshakeError`/`ErrIncompatibleVersion`); always `errors.Join(net.ErrUnableToConnect, …)`; nodeconf switch → `errors.Is`; negative cache 60s on clients; `pick` fix in PR-A. |
| M2 | blocking + slow-net | v1 compressed time-to-success, not time-to-failure: joined errors wait for the slowest candidate (quic wall = **2×**`HandshakeIdleTimeout` = 20s, + fresh 10s post-handshake window = 30s), ×3 sequential nodes ≈ 90s to a failure verdict (worse with C2: ~6 min). | D2 soft-cap ~8s: Dial returns joined errors and detaches stragglers (safe under D1); GetOneOf staggers nodes ~2s; offline verdict ≤ ~12s. |
| M3 | slow-net | 250ms stagger < 600ms RTT ⇒ every cold connect is a full 3-way fan-out: 3 complete handshakes/node (2 discarded), 3× server accept amplification, radio/metered cost; wrong-winner risk (yamux beats quic on the same slow path; yamux 10s `StreamOpenTimeout`/`ConnectionWriteTimeout` then kills whole sessions under bufferbloat). | Phase-split (C1 fix) removes the handshake amplification (losers = cheap connects); grace window (C10) addresses wrong-winner; "quic won last time" per-epoch bit promoted from optional to required for high-RTT links. |
| M4 | concurrency | Under D1, `pool.Flush` neither cancels in-flight dials (`ForEach` skips loading entries) nor stays non-blocking (`Remove` waits on `waitLoad` — up to the full window on the RPC thread), removes by id not `RemoveSame` (can close the fresh post-recovery conn), and a pre-switch dial installs a dead conn into a just-flushed pool. | D1 spec: per-entry cancel handles + pool generation counter; Flush cancels loads, uses `RemoveSame`, never blocks on loading entries. |
| M5 | concurrency | RWMutex wedge chain: ranker state read under `peerService.mu` across a long walk starves `SetPeerAddrs` writers → D7's hook (serial under `hookMu`, shared with `StateChange`) blocks → **foreground RPC freezes** up to the dial window. | Lock discipline in D2: snapshot under short RLock, dial lock-free, record outcomes under a fine-grained mutex; D7 hook collects under `hookMu`, applies after release. |
| M6 | concurrency | Local-peer fan-out unbounded: mDNS advertises one addr per interface (VPN/docker/VM bridges included) × 30ms stagger ≈ simultaneous; 5 stale peers × 8 addrs = 80 sockets (dials+probes) held 30s on the radio — violates §5.1's "locals never compete with nodes". | D2: ≤3 in-flight per Dial; candidate-list cap for discovery-sourced addrs; shorter local window (C8). |
| M7 | concurrency + local-only | D7's "vanish instantly" was overstated: addrs snapshotted at Dial start; nothing cancelled in-flight dials; pool entries survive `SetPeerAddrs(nil)` for up to TTL 1min; pool↔peerstore diverge with no reconciler while the p2p UI counts peerstore entries. | D7: invalidation = peerstore prune + `SetPeerAddrs(nil)` + `pool.Remove` + cancel in-flight (D1 handles) + epoch re-check before caching a winner. |
| M8 | rollout | `preferYamuxTransport` semantics: today = "quic only after all yamux failed" (operationally: quic off); a 250ms-anchor ranker degrades the incident-survivors' escape hatch to a hint. | D2: `preferQuic=false` keeps **exclusion** semantics (quic only after all yamux candidates terminally fail). |
| M9 | rollout | Heart cannot roll back a client release; no kill switch existed in v1. Also: netcheck.go implements `nodeconf.NodeConf` (0.13-bump compile break) and already ships `DialTimeoutSec: 60` — D3 v1 silently double-loaded that knob. | PR-C ships a runtime ranker kill switch (fallback = sequential walk); post-handshake window gets its **own** config field; netcheck stubs listed in the (eventual) 0.13 bump checklist. |
| M10 | concurrency | `evictOnClose` armed inside the loadFunc before `e.value` is stored → `RemoveSame` no-ops for conns dying in that window (made likely by racing); `watchPeer` registration skippable for a replacement peer with the same id (death then unnoticed ≤2min); `RegisterSpace`/`UnregisterSpace` reorder under D8 retries leaks phantom p2p status. | D1 `onStored` hook for eviction arming; heart fixes: watchingPeers keyed by peer identity, register/unregister ordering — bundled into heart-2. |
| M11 | slow-net | Backoff-vs-cadence resonance (5s base == 5s fast-retry) makes A/B measurements irreproducible; winner's peer-wide backoff clear races losers' failure records (monotonic creep on the losing scheme). | D5: base 3s; per-addr (not peer-wide) clear applied as the final write of a Dial; cancelled losers neutral (C10). |
| M12 | rollout | PR-A/B v1 straddled the only two files that diverge between lines (webtransport, ocache) while the trivially-portable six needed nothing; PR-B was specced against 851d38f4 logic that exists only on v0.13.x. | §5: PR-A split into mechanical cherry-pick + per-line webtransport cap; ocache work specced per line (main has no abort-retry to reconcile). |

## Minor (still scheduled)

- Probe hardening: ctx-cancellable `Probe` (today blocks in `Read` on a socket
  deadline only), own 300ms budget inside the dialer, ≥2 refusals before
  `Dead`, demote-then-verify instead of immediate removal (unauthenticated
  ICMP on hostile LANs). Windows path ships dark (`Inconclusive`) until
  real-Windows validation; relocation split into its own PR.
- Wall-clock-vs-suspension: discard dial outcomes when the clock-jump
  detector fired mid-dial; reset statistics (cheap) even for <15s
  backgrounds.
- Broadcast peer-find deadline (30s) ≥ dial window with only 4 streampool
  workers → shrink to ~5s once D2 lands.
- `pool.Get`'s transient loading entry in the **incoming** cache can make
  `AddPeer`'s recovery path close a healthy inbound conn — add a guard.
- Server accept goroutines parked ≤10s by cancelled losers: accepted cost,
  bounded; noted for server capacity review.

## What the review left standing

Timeout budgets at 600ms RTT (any-sync handshake = 2 RTT; 10s windows are
generous); dropping the 3s cap; `SetPeerAddrs(nil)`-as-clear verified against
the pinned v0.12.16 byte-for-byte; LocalOnly node path benign (empty node
set fails fast, UI short-circuits before reading nodeStatus); offline cadence
does not starve LAN sync; single-flight fan-out arithmetic (per-peer, not
per-space); inbound accept phases all bounded once the zero-guard lands —
which is itself a **server-side bug fix** (see R0), not just a dial-side
guard.

---

# Meta-review (Fable, 2026-08-03) — audit of the v2 resolutions

**Verdict:** v2 architecturally ready; 17 of 22 critical/major resolutions
verified real (branch-topology and byte-identical-files claims re-verified by
git diff; the phase-split seam confirmed feasible at `quic.go:108-165`).
Residual defects concentrate in the subsystems v2 *added* (D9 liveness, D4
aggregation, the D1/D2 straggler seam). All fixes below are folded into the
design doc, marked "meta-review F*".

| # | sev | finding | disposition |
|---|---|---|---|
| F1 | major | D4's "any UDP success → Allowed" re-admits the post-handshake-kill trap: every dial *succeeds*, so success/early-death pairs arrive 1:1 and Blocked never engages. | D4: successes provisional — count only after ~60s survival / inbound stream activity; early death retro-converts its own success sample. |
| F2 | major | D9 serve-gating measured the wrong layer: quic `BytesRead` = stream payload only (PINGs invisible, `conn.go:202-208`), yamux counts its own keepalives; and a 15s idle gate over a 12-15s transport idle timeout can only fire on **healthy idle** quic conns. | D9: gate respecced request-scoped (outstanding request, no response bytes > ~2× write timeout); idle conns left to the transport idle timeout. |
| F3 | major | Straggler ownership hole: on the 8s soft-cap error, ocache **deletes the entry** (`ocache.go:210-222`) — "a late success is cached" had no mechanism, and routing it via `AddPeer` re-enters the C1 duplicate contract. | D2.7: soft cap releases *waiters* while the load stays open under its window/generation; late success completes into the still-live entry; Flush's cancel handle covers it. |
| F4 | major | Hung-handshake (R0-shape) server gets no within-Dial fallback: 8s soft cap < 10s serialized handshake budget, and the "quic won" bit is set by a phase-1 win, re-anchoring the trap every cycle. | D2.3: per-handshake budget ≈ min(4s, softCap/2) with promote-on-budget; the quic-won bit set only on full dial success. |
| F5 | major | PR-A's naive `AddPeer` keep-existing inversion bounces app-restart/NAT-rebind reconnects for up to ~30s against a dead-but-undetected server-held conn. | Moved to PR-D with a liveness tiebreak (keep existing only when demonstrably alive); PR-A keeps today's keep-newest; the phase split alone covers C1's dial-race duplicates. |
| F6 | major | The M1 fix list missed `pool.GetOneOf`'s own bare type assert (`pool.go:163`), which flattens the unanimous classification before nodeconf ever sees it. | D2.6: `errors.As` + joined pass-through in the same commit as the nodeconf fix; both in the compat test. |
| F7 | minor | Early-death events from pre-switch conns pollute the new epoch's stats. | D9: events epoch-stamped at establishment; cross-epoch discarded. |
| F8 | minor | `MaxIdleTimeoutSec ≈ 12s` churns healthy 600ms-RTT lossy links (effective keepalive 6s; PTO chains can exceed the window) and each churn feeds failure stats. | D3/D9: 15s; feeding D5 requires ≥2 consecutive idle deaths on the same addr. |
| F9 | minor | Fixed 500ms grace under-serves >500ms-RTT links — M3's wrong-winner survives where it hurts most. | D2.3: grace = max(500ms, ~1× smoothed per-epoch handshake time). |
| F10 | minor | D7's "TTL ↓ and/or browse restart" must be **and** — TTL heals the remote's view, browse restart heals ours. | D7: both mandatory; TTL-120s radio cost stated as accepted. |

Resolution audit: C1-C10 resolved (C3 partial→closed by F1/F2, C7 contingent→
closed by F10); M1 partial→closed by F6; M2/M4 residuals closed by F3/F4;
M3 partial→mitigated by F9; M9 partial→closed by the sharpened kill-switch
spec (togglable without app update; fleet-wide remote disable acknowledged as
needing the nodeconf/coordinator refresh channel if wanted). Completeness for
the three standing scenarios: local-only covered; slow networks covered
modulo F8/F9 parameter choices; partial-blocking covered once F1/F2 land —
late kills (>60s) acceptably handled by the 15s idle floor plus redial.

---

# Second focused review (Fable, 2026-08-03) — five unexamined lenses

Lenses: **L1 hostile-LAN / adversarial peer**, **L2 mobile resource
economics**, **L3 two-phase Transport API shape**, **L4 observability & field
validation**, **L5 post-edit spec coherence**. No lens came back clean. The
defect class the prior rounds could not see: v2 adds long-lived learned state
(BH window, per-(peer,addr) backoff, epoch tags, smoothed handshake time) and
a 30× faster LAN discovery loop on top of a trust model where every LAN input
— mDNS announcements, `SpaceExchange.LocalServer.Ips`, ICMP, and the
interface-address delta that gates the epoch — is unauthenticated and
forgeable. All amendments folded into the design doc, marked "G*".

| # | sev | lens | finding | disposition |
|---|---|---|---|---|
| G1 | critical | L2 | mDNS TTL 3600→120 is mispriced: `sentEntries` dedup expires with the TTL, so each LAN peer is **re-delivered every ~2min** and `PeerDiscovered` runs a full dial + `SpaceExchange` RPC per delivery with no dedup (~10 RPC/min, idle phone, 20-peer office). Not "chatter". | D7: mandatory — suppress the PeerDiscovered action path when the peerId is already pooled with an unchanged addr set; TTL drop must not ship without it. |
| G2 | critical | L1 | One spoofed mDNS announcement (Instance=victim peerId) displaces the victim's whole addr list (`SetPeerAddrs` is a replace, peer.go:15/peerservice.go:185); two forged ICMP evict it (`Dead`→`RemoveLocalPeer`, forgeable on-LAN). AddPeer displacement is infeasible (peerId cryptographically bound at handshake). | D7: discovery writes = per-addr **union, never replace**; never remove addrs of a peer with a live pooled conn. D2.4: probe `Dead` demote-and-verify for **all** classes incl. local; never destructive eviction evidence alone. |
| G3 | critical | L1 | A LAN attacker churning the interface-address delta (rogue RAs / DHCP) keeps the epoch bumping → erases BH/backoff/quic-won/smoothed-RTT (permanent cold start) + `pool.Flush` every 6s. v2.0 elected the most forgeable signal as the erasure authority. | D7: epoch damped (delta persists ≥2 ticks, ~30s min spacing, exp damping). D4: epoch change **invalidates (stale, re-verify) rather than erases**, keeping a cross-epoch prior. |
| G4 | major | L2 | D7's demote-not-delete keeps `LocalPeerIds` non-empty, disabling the offline 2-min cadence gate (`len==0`) — phone dials demoted unroutable peers at 20s on cellular forever. Second gate (`IsOffline`) already fragile on mobile. | D7: offline gate counts only epoch-current, non-demoted peers; demoted peers not dialed while offline. |
| G5 | major | L2 | "Discard suspension-tainted outcomes" races the recording it gates (detection lags resume ≤1 tick; sub-30s iOS freezes invisible) and resume-time idle deaths of pre-background conns feed same-epoch failure samples. | D4: per-event wall+monotonic stamp on each dial **and** establishment; discard on >1s drift — thresholdless, catches sub-30s freezes. |
| G6 | major | L1 | Discovery state unbounded and attacker-declared: no global cap on discovered peers / dial concurrency / backoff-table cardinality; `SpaceExchange` leaks all space ids and stores self-declared membership; `LocalServer.Ips` unvalidated (amplification / internal-port targeting). | New D10: global LRU + device-wide dial cap; table cardinality+TTL bounds; validate Ips vs remote subnet; note SpaceExchangeV2 (v0.13) is the real fix the v0.12.20 decoupling defers. |
| G7 | major | L3 | Transport split is at the wrong seam (secureservice already separates libp2p-TLS from any-sync handshake → yamux gets phase-1 identity too), mismodels webtransport (identity is tri-state, not binary), and M3's "losers are cheap" is false for quic (phase 1 = full server TLS). Likely over-engineered vs a ctx handshake-permit. | D2: prototype the ctx-permit variant first (cancelled loser = `context.Canceled` = neutral, zero interface change); if a split is chosen, seam inside secureservice; spec identity tri-state; honest cost correction. |
| G8 | major | L4 | Six new state machines, zero specified observability; all three field channels closed by default (logs `*=WARN`, prometheus never exported by heart, telemetry has no net events); debugstat is the only support channel and peerservice/transports register nothing. | New D10: `net.peerservice` debugstat provider (BH windows, backoff table, the six counters); `peer.Stat` + `{scheme,addr,phase1Ms,handshakeMs,epoch}`; `net.peerservice=INFO` default. |
| G9 | major | L4 | Nothing validates BH/backoff against production traffic; PR-A's zero-guard silently normalizes the R0 misconfig you need to find, and accept aborts log no elapsed time. | D10: ship D4/D5 in **shadow mode** (compute+export, don't act) then enable; startup WARN naming the defaulted field; stamp accept errors with elapsed time so ~0ms aborts are visible. |
| G10 | major | L5 | Post-edit rot: D3 still says 12s (F8 set 15s); §1 bug table says "all fixed in PR-A" (F5 moved AddPeer→PR-D); heart-2's 5s broadcast deadline is now < the 8s soft cap; 15s idle → 7.5s keepalive battery cost unstated. | Doc fixed: 15s everywhere with (see D9); per-row PR targets on the §1 table; broadcast deadline → ~10s (≥ soft cap); keepalive cost stated in D9. |
| G11 | major | L5 | D7 eviction needs "probe Dead / terminal error" evidence heart can't observe after PR-C2 relocates the probe + D2.6 flattens errors to one sentinel. | D2.6: join a typed `ErrAllAddrsDead` alongside the sentinel so heart can `errors.Is` it. |
| G12 | minor | L5 | F9 grace = smoothed **per-epoch** handshake time is empty on the first post-switch dial (F9's own case) → sits at the 500ms floor; Risk 5's "≤2min" unblock is stale after F1's provisional successes. | D2.3: seed grace from cross-epoch prior / phase-1 timings. D4/Risk 5: unblock = MinSuccesses confirmed; realistic recovery 2-4min. |

Net: no architectural reversal, but the design now carries a new item (D10:
observability + shadow rollout + adversarial hardening) and the security
posture is explicit — the v0.12.20 decoupling that avoids the protocol
migration also **defers SpaceExchangeV2**, so the LAN trust gap is widened by
the faster discovery loop until the caps in D10 land. That trade-off is now
stated rather than implicit.
