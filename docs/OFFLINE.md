# Offline mode implementation

## What has been done

- Removed the quic-go/yamux/webtransport transports from the mobile dependency graph by replacing their registrations with `space/spacecore/offlinenet` stubs and patching the any-sync fork's `peerservice`/`quicdemotion` to stop importing the transport packages.
- Replaced the heart-local LAN server (`space/spacecore/clientserver`) and local discovery with offline no-ops.
- Neutered network diagnostics (`core/debug/netcheck.go`).
- Removed `GetQuic`/`GetYamux` from `core/anytype/config/config.go` (they were only used by the real transports).
- Forced `Config.IsLocalOnlyMode()` to always return `true`.
- Bounded `idresolver.ResolveSpaceIdWithRetry` to 10 attempts (was unlimited), fixing the media loading hang when the objectstore index is missing.
- Installed a loopback-only `http.DefaultTransport` in `core/anytype/offlinenet/http.go` so any remaining code path that tries to reach the network fails fast at runtime.
- Added a per-session random gateway token (`AuthToken`) and removed CORS/listing from the gateway.
- Added `scripts/verify-offline.sh` to enforce the above.

## Known residual (P2 follow-up)

- Several packages still contain HTTP/dial call sites (linkpreview, unsplash, gallery, bookmark, configfetcher, publish, pushnotification, payments, identity, AI, fileuploader, metrics/anymetry, builtinobjects gallery download, localdiscovery self-connect). They are listed as pending in `scripts/verify-offline.sh` and are blocked at runtime by the loopback-only transport. The long-term fix is to no-op or delete each of these features (P2/P4).
- `libp2p/core/...` crypto/peer/peerstore/record packages remain in the dependency graph because any-sync uses them for identity keys and peer IDs. These do not establish network connections.
- `go list -deps` cannot target `GOOS=android` directly because `tantivy-go/internal` is CGO-only; the script uses `GOOS=linux` and documents the `zeroconf`/`mdns` packages as android-excluded.
