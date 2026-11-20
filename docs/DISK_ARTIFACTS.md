# On-disk artifact inventory (initial recon, P0)

Status: initial. P2/P6 must complete exact paths + WAL/shm/lock coverage and verify nothing user-derived stays plaintext.

| Artifact | Location (today) | Encrypted today? | Target protection (P6) |
|---|---|---|---|
| Object store DBs (any-store v2) | RepoPath() per-space dirs (pkg/lib/localstore/objectstore) | NO | any-store v2 Config.Encryption / custom Codec (XChaCha20-Poly1305 if supported, else AES-256-GCM native) |
| any-store v1 DBs (any-sync space storage) | via any-sync v0.13.8 (any-store v1.0.3) | NO | backport/upgrade encryption in aparok-store fork + replace in aparok-sync, or encrypting wrapper |
| WAL / shm / lock sidecars | next to each DB | NO | covered by page-level encryption; verify no plaintext leakage in WAL |
| File blocks (flatfs) | RepoPath()/FlatfsDirName (core/files/filestorage/fileservice.go:101, flatstore.go) | NO | encrypting wrapper: per-block XChaCha20-Poly1305, random nonce, AAD=block id |
| Tantivy full-text index | core/indexer (fulltext.go) - exact dir TBD | NO (plaintext index) | RAM dir rebuilt on unlock if supported; else patched tantivy-go fork or any-store encrypted fulltext |
| Chat data | object store + indexer (core/block/chats) | NO | same as object store + index |
| Thumbnails / image cache | app-side + gateway cache (TBD) | NO | Tink StreamingAead (P7) / encrypted core cache |
| Logs / crash dumps | core logs + app logs (TBD) | NO | encrypted log sink; strip in release |
| Temp / staging | RepoPath tmp dirs (TBD) | NO | encrypted or memory-only; wiped on lock/exit |
