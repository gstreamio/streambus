# raft v3.7.0 Migration Notes

**Status: not undertaken.** Dependabot opened a PR bumping `go.etcd.io/raft/v3` from
`v3.6.0` to `v3.7.0`; it failed CI and was closed. This document records what an
engineer actually attempted (a full migration, reaching a clean build and a passing
test suite matching the pre-existing baseline) and why the work was reverted rather
than merged, so the investigation doesn't have to be repeated the next time this
dependency comes up.

`go.mod`/`go.sum` are back on `v3.6.0`. `pkg/consensus/{node.go,storage.go,
transport_v2.go,types.go}` and their test files are unchanged from `main`.

## Why we looked at this at all

Dependabot's PR (#33 by reference) proposed the bump. Nothing else prompted it — no
bug report, no feature we were blocked on, no incident. That matters, because the
cost of this migration (below) is only worth paying if there's a concrete reason to
pull in the newer version.

## What changed in v3.7.0, and when

- Released 2026-06-03 (tag `v3.7.0`, commit `b867cf13f6bc0dae21204302df97bc2355c3af55`,
  upstream `https://github.com/etcd-io/raft`). Confirmed via the module's own
  `@v/v3.7.0.info` metadata in the local module cache.
- The shipped `CHANGELOG/CHANGELOG-3.7.md` lists exactly three changes:
  - [Allow users to pass in a snapshot with only the ConfState initialized during bootstrap](https://github.com/etcd-io/raft/pull/370)
  - [Improve the ReadIndex flow to prevent stale read index caused by RequestIndex retries](https://github.com/etcd-io/raft/pull/397)
  - [Align formatting of node ID in MajorityConfig](https://github.com/etcd-io/raft/pull/414)
- **The changelog does not mention the change that actually breaks every consumer**:
  `raftpb` was regenerated against `google.golang.org/protobuf` instead of
  `gogo/protobuf`. The generated file headers confirm this directly:
  `go.etcd.io/raft/v3@v3.7.0/raftpb/raft.pb.go` now reads
  `// protoc-gen-go v1.36.11` where the v3.6.0 tree used gogo's generator. `go.mod`
  correspondingly drops `github.com/gogo/protobuf` and `github.com/golang/protobuf`
  as indirect dependencies once the bump is applied. This is an undocumented
  breaking change riding along in a semver-minor release — worth remembering next
  time a "minor" bump from this module fails CI; it may not be minor at all.
- No CVE or security advisory exists against `go.etcd.io/raft/v3` at any version.
  (There are several real advisories against `go.etcd.io/etcd/server/v3` — the etcd
  *server*, a much larger module that happens to embed raft internally alongside
  RBAC, gRPC, and storage — but none of those name the standalone `raft/v3` module
  we import, and none apply to a library-only consumer like StreamBus that doesn't
  use etcd's server package at all.)

## The concrete API shifts

All of the following stem from the single gogo → google.golang.org/protobuf
regeneration, not from independent design decisions in v3.7.0:

1. **Scalar fields on every raftpb message become pointers**, with nil-safe `GetX()`
   accessors generated alongside. `raftpb.Entry.Index`/`.Term`/`.Type` are now
   `*uint64`/`*uint64`/`*EntryType`; `raftpb.Message.To`/`.From`/`.Term` are now
   `*uint64`; `raftpb.HardState.Term`/`.Vote`/`.Commit` are now `*uint64`;
   `raftpb.SnapshotMetadata.Index`/`.Term` are now `*uint64`. Consequences:
   - `entry.Index <= rn.appliedIndex` no longer compiles (`*uint64` vs `uint64`);
     needs `entry.GetIndex() <= rn.appliedIndex`.
   - `switch entry.Type { case raftpb.EntryNormal: ... }` no longer compiles
     (comparing `*EntryType` against an untyped `EntryType` constant); needs
     `switch entry.GetType() { ... }`.
   - Struct literals like `raftpb.Message{To: 2}` no longer compile — `2` can't
     implicitly become `*uint64`. Building one now needs either
     `raftpb.MsgHeartbeat.Enum()` (constants gain an `.Enum()` method returning a
     pointer to a copy) or `proto.Uint64(2)`
     (`google.golang.org/protobuf/proto` ships `Bool`/`Int32`/`Int64`/`Uint32`/
     `Uint64`/`String`/`Float32`/`Float64` helpers for exactly this).
   - `raft.IsEmptyHardState`/`raft.IsEmptySnap` are already nil-safe against the new
     pointer types (`st == nil || isHardStateEqual(...)`, and
     `sp.GetMetadata().GetIndex() == 0` chains through nil-safe getters), so the
     existing guards in `node.go`'s `run()` loop don't need new nil checks — they
     already do the right thing once the surrounding signatures are updated. This
     was the one place I went in expecting to need new guards and didn't.
2. **Slices and several parameters switch from value to pointer element/argument
   types.** `raft.Storage.Entries` must return `[]*raftpb.Entry`, not
   `[]raftpb.Entry`; `raft.Storage.InitialState` must return
   `(*raftpb.HardState, *raftpb.ConfState, error)`; `raft.Node.Step` takes
   `*raftpb.Message`; `raft.Ready.Entries`/`.CommittedEntries` are
   `[]*raftpb.Entry`; `raft.Ready.HardState`/`.Snapshot` are
   `*raftpb.HardState`/`*raftpb.Snapshot` (this is also where nil becomes
   representable where it structurally could not be before).
3. **`raftpb.ConfChange`**: `NodeID` is renamed `NodeId` (mechanical); `Type` becomes
   `*ConfChangeType` (needs `.Enum()`); and the type no longer satisfies
   `raftpb.ConfChangeI` by value — only `*ConfChange` does (`AsV1`/`AsV2` have
   pointer receivers), so every construction site needs `&raftpb.ConfChange{...}`.
4. **Generated `Marshal()`/`Unmarshal()`/`Size()` methods are gone.** gogo/protobuf
   attached these directly to the message types; the new codegen doesn't. Call
   sites become `proto.Marshal(msg)` / `proto.Unmarshal(data, msg)` /
   `proto.Size(msg)` from `google.golang.org/protobuf/proto`.

## The copylocks hazard (the reason this isn't a "just add pointers" job)

Every generated message type now embeds `protoimpl.MessageState`, which in turn
embeds `pragma.DoNotCopy`. That type is defined as:

```go
// google.golang.org/protobuf/internal/pragma/pragma.go
type DoNotCopy [0]sync.Mutex
```

It's a zero-length array of `sync.Mutex` — zero runtime cost, but `go vet`'s
copylocks analysis treats any type containing it as "contains a mutex," and flags
any copy by value. I verified this is not theoretical by reproducing the original
(pre-migration) code's exact value-semantics signatures — `Send(msg raftpb.Message)`,
`sendMessage(msg raftpb.Message)`, `handleMessage(msg raftpb.Message)`,
`applySnapshot(snap raftpb.Snapshot)`, `SaveSnapshot(snap raftpb.Snapshot)`, and a
plain `for _, e := range entries` over a `[]raftpb.Entry` — against raft v3.7.0 in an
isolated probe module and running `go vet`:

```
main.go:6:15: Send passes lock by value: go.etcd.io/raft/v3/raftpb.Message contains google.golang.org/protobuf/runtime/protoimpl.MessageState contains sync.Mutex
main.go:7:21: call of sendMessage copies lock value: ...
main.go:10:22: sendMessage passes lock by value: ...
main.go:11:6: assignment copies lock value to _: ...
main.go:16:24: handleMessage passes lock by value: ...
main.go:17:6: assignment copies lock value to _: ...
main.go:22:9: range var e copies lock: go.etcd.io/raft/v3/raftpb.Entry contains ... sync.Mutex
main.go:23:7: assignment copies lock value to _: ...
main.go:28:25: applySnapshot passes lock by value: go.etcd.io/raft/v3/raftpb.Snapshot contains ... sync.Mutex
main.go:29:22: call of saveSnapshot copies lock value: ...
main.go:32:24: saveSnapshot passes lock by value: ...
main.go:33:6: assignment copies lock value to _: ...
```

Every one of those signatures exists in `pkg/consensus` today, essentially
unchanged since the code was first written around value semantics. `go vet` (which
already runs in this repo's normal build) would flag every one of them the moment
the dependency bumps, which is at least a usable safety net for the *sites vet can
see*. The actual risk is the sites it can't: a copy made through an `interface{}`
value, a channel send of a value type, a value captured in a closure, or anything
reached via reflection won't be caught by copylocks, and would still compile and
run — just with a raftpb message silently sharing (or not sharing, depending on
where the copy happens) a `[0]sync.Mutex` that exists purely to make static analysis
angry, not to provide real synchronization. In practice this ends up mattering less
for correctness than it sounds (the mutex is zero-size and never locked/unlocked by
anything in this codebase), but it's a marker for "this value was not supposed to be
copied," and a consensus implementation with a hundred-plus call sites that all need
individual judgment calls about whether a given copy is safe is not a low-risk
place to lean on "vet mostly catches it."

## Scope: how big is this, actually

Measured with `go build -gcflags="-e" ./pkg/consensus/...` (production code) and
`go test -gcflags="all=-e" -c -o /dev/null ./pkg/consensus/` (test code), both of
which disable the compiler's normal 10-error cutoff so the count isn't an
artifact of early truncation:

| File | Distinct compiler errors |
|---|---|
| `node.go` | 36 |
| `storage.go` | 21 |
| `transport_v2.go` | 3 |
| **production subtotal** | **60** |
| `storage_coverage_test.go` | 88 |
| `transport_coverage_test.go` | 63 |
| `node_test.go` | 22 |
| `node_coverage_test.go` | 19 |
| `transport_v2_test.go` | 35+ |
| **test subtotal** | **227+** |
| **total** | **287+** |

Methodology note on the `35+`: the other four test-file counts were measured after
production code was already fixed (so the compiler could fully resolve the new
signatures test code calls into); `transport_v2_test.go`'s count was captured in a
follow-up isolated re-measurement where production code was *not* yet fixed, which
under-reports (the compiler can't finish resolving call sites into a callee that
itself fails to compile). The real number for that file, measured the same way as
the other four, is very likely somewhat higher — I'm flagging the inconsistency
rather than presenting a falsely-precise total. Either way, "roughly 64 call sites"
(the estimate that shaped the original task) undercounts the true scope by a factor
of four to five once test files — which construct raftpb literals just as directly
as production code — are included.

## Test files are part of the migration, which undermines the safety net

`node_test.go`, `node_coverage_test.go`, `storage_coverage_test.go`,
`transport_coverage_test.go`, and `transport_v2_test.go` all construct raftpb values
directly (`raftpb.Message{To: 2, ...}`, `raftpb.Entry{Index: 1, ...}`, helper
functions like `makeHardState`/`makeEntries`) and would need every one of the same
pointer-conversion edits as production code. That means "the tests pass" cannot be
used as independent validation that the migration is correct — the tests are being
rewritten in the same diff, by the same reasoning, and can just as easily encode
the same mistake as the production code they're checking. This is worth calling
out concretely rather than abstractly, because it actually happened during the
attempt:

- `assert.Equal(t, uint64(0), someStruct.Field)` — where `Field` had been converted
  to a pointer — **compiles successfully** (testify's `Equal` takes `interface{}`,
  so a `uint64` and a `*uint64` box up fine as arguments) but then fails at
  **runtime**, unconditionally, regardless of the pointed-to value, because
  `reflect.DeepEqual` never considers a `uint64` and a `*uint64` equal. Found at
  five call sites across three test files by grepping for the six affected field
  names after finishing the mechanical pass, and separately during an actual test
  run. This is exactly the "compiles but wrong" hazard mentioned as the danger of
  this migration, reproduced not in the consensus hot path but in the tests meant
  to catch problems in it.
- `TestDiskStorage`'s "initial state" subtest did `hs.Term` — direct field access,
  no accessor — on the `*raftpb.HardState` returned by a fresh, never-written
  `DiskStorage`. That value is legitimately `nil` now (mirroring upstream's own
  `MemoryStorage`, which represents "no HardState persisted yet" as a nil pointer
  rather than a zero-value struct). Direct field access on a nil pointer panics;
  `hs.GetTerm()` doesn't. This one didn't just fail a test — it panicked the whole
  test binary (SIGSEGV), and would only have been caught by actually running the
  suite, not by getting it to compile.

Neither of those is hypothetical residue from an incomplete attempt — both were
present in a version of the migration that built cleanly, passed `golangci-lint`
with zero issues, and matched `main`'s pre-existing three-test failure baseline
(`TestThreeNodeCluster`, `TestNode_CreateSnapshot`, `TestNode_ConfChange_RemoveNode`)
across two consecutive clean test runs. A green build and a matching test baseline
were not sufficient evidence that the migration was correct, which is the central
reason this wasn't merged.

## Places that needed a judgment call about aliasing vs. copying

These are the specific spots where converting a value field to a pointer field
meant deciding what the pointer should point *at* — get this wrong and it compiles,
lints clean, and is wrong at runtime, which is precisely the failure mode this
document exists to flag:

- **Snapshot construction in `createSnapshot()`.** The naive fix for
  `raftpb.SnapshotMetadata{Index: rn.appliedIndex, ...}` (now `Index *uint64`) is to
  write `Index: &rn.appliedIndex`. That's wrong: it aliases the snapshot's
  serialized index directly into `RaftNode`'s live, mutating field. If
  `rn.appliedIndex` changes after the snapshot is built but before it's actually
  written/marshaled, the "already captured" snapshot would silently observe the
  new value instead of the one at capture time. The correct fix copies into a
  fresh local (`snapIndex := rn.appliedIndex`) and takes the address of the copy,
  not the field.
- **`AddNode`/`RemoveNode`'s `ConfChange.NodeId *uint64`.** Here taking the address
  of the `nodeID` function parameter directly (`&nodeID`) is fine, not a bug — a
  function parameter is a private, non-shared copy for the duration of the call,
  unlike a struct field that lives on and mutates after the call returns. The
  distinction that matters is addressability *and* who else can see the value change,
  not addressability alone.
- **`raftpb.ConfChangeType`/`MessageType`/`EntryType` constants.** These can't be
  addressed directly (`&raftpb.ConfChangeAddNode` doesn't compile — constants
  aren't addressable), which is exactly why the generated `.Enum()` method exists:
  it returns a pointer to a fresh copy of the constant's value, sidestepping the
  addressability problem instead of working around it with a manually-declared
  local variable at every call site.
- **`DiskStorage`'s internal representation.** The natural-looking fix keeps
  `DiskStorage`'s own fields as values internally and only converts at the
  `raft.Storage` interface boundary. I didn't do that — I changed
  `DiskStorage.hardState`/`.confState`/`.snapshot`/`.entries` to the same pointer
  types raft.Storage exposes, matching upstream's own `MemoryStorage`. The
  alternative (value fields, pointer-izing only at the boundary) would silently
  collapse "no HardState persisted yet" (legitimately nil) into a zero-value
  `HardState{}` on every read — indistinguishable from "a HardState of all zeros was
  explicitly persisted," which is exactly the kind of nil-information loss this
  migration is supposed to avoid, not reintroduce at a translation seam.

## What would need to be true to take this on

- **A concrete reason to move**, not "a newer version exists." Something in a
  release we actually need (a fix, a feature, a security advisory) — none of which
  is currently true for `v3.7.0` against our `v3.6.0`.
- **A test strategy that doesn't move with the production code.** Since the test
  files construct raftpb values as directly as production code does, "the existing
  tests pass" can't be the bar. That likely means: convert the tests first, in their
  own reviewable diff, confirm they still assert the same behavior against the
  *old* dependency (i.e., prove the test conversion alone is behavior-preserving),
  and only then flip the dependency version — so the production-code diff is
  reviewed against tests already known to be trustworthy, rather than trusting a
  batch of new tests and new production code to have gotten the same pointer
  conversions right in tandem.
- **A line-by-line review budget for every aliasing decision**, not just a green
  `go vet`. The copylocks findings are a useful tripwire for the subset of sites
  they can see (direct value copies through named functions and range loops), but
  they don't cover copies through `interface{}`, channels, closures, or reflection,
  and they say nothing at all about the aliasing-vs-copying judgment calls above,
  which are a correctness question `go vet` isn't designed to answer.
- Given none of the above is currently in place and there's no external driver,
  the recommendation is to leave this dependency on `v3.6.0` and revisit only if a
  specific need appears in a future raft release.
