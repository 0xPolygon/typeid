# typeid

Prefixed, base32-encoded, k-sortable identifiers for Go. Inspired by [Stripe API IDs](https://stripe.com/docs/api) and the [TypeID spec](https://github.com/jetify-com/typeid).

[![GoDoc Widget](https://godoc.org/github.com/go-chi/typeid?status.svg)](https://pkg.go.dev/github.com/go-chi/typeid)
[![Apache 2.0 License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE-APACHE)
[![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE-MIT)

## Identifier format

```
user_01kmfjypewe1wrfeb01wjfxand       UUID  — 26-char suffix
└──┘ └────────────────────────┘
type   Crockford base32

org_01kmfjypewdwg                     Int64 — 13-char suffix
└─┘ └───────────┘
type  Crockford base32
```

The alphabet is Crockford base32 (lowercase) and excludes ambiguous characters: `i`, `l`, `o`, `u`.

## Two flavours

Both are UUIDv7-based and sort by creation time (no UUIDv4 — sortability gives good DB locality and time-ordered IDs).

| Type | Backing | Postgres | Suffix | When to use |
|------|---------|----------|--------|--------------|
| `UUID[P]` | 128 bit | `uuid` | 26 chars | Any throughput. Users, events, logs — use by default. |
| `Int64[P]` | 63 bit | `BIGINT` | 13 chars | &lt;~100 IDs/sec. Orgs, tenants — compact IDs, 15 random bits; use UNIQUE + retry on conflict. |

Currently, the UUID type is backed by `github.com/google/uuid` but we plan to switch to [Go's `uuid` package.](https://github.com/golang/go/issues/62026) once available. This will likely be a breaking change before we release v1.

## Usage

### Define typed IDs

```go
import "github.com/0xPolygon/typeid"

type userPrefix struct{}
func (userPrefix) Prefix() string { return "user" }

type UserID = typeid.UUID[userPrefix]

type orgPrefix struct{}
func (orgPrefix) Prefix() string { return "org" }

type OrgID = typeid.Int64[orgPrefix]
```

### Create new IDs

```go
userID, err := typeid.NewUUID[userPrefix]()   // user_01kmfjypewe1wrfeb01wjfxand
orgID,  err := typeid.NewInt64[orgPrefix]()   // org_01kmfjypewdwg
```

### Parse from string

```go
id, err := typeid.ParseUUID[userPrefix]("user_01kmfjypewe1wrfeb01wjfxand")
id, err := typeid.ParseInt64[orgPrefix]("org_01kmfjypewdwg")
```

Parsing validates the prefix at compile time — passing `"org_..."` to `ParseUUID[userPrefix]` returns an error.

### Wrap raw values

```go
id, err := typeid.UUIDFrom[userPrefix](rawUUID)   // rejects non-UUIDv7
id, err := typeid.Int64From[orgPrefix](rawInt64)   // rejects non-positive
```

### Use in structs

```go
type User struct {
    ID   UserID `json:"id"`
    Name string `json:"name"`
}

type Org struct {
    ID   OrgID  `json:"id"`
    Name string `json:"name"`
}
```

## Serialisation

Both types implement:

| Interface | Behaviour |
|-----------|-----------|
| `fmt.Stringer` | `"prefix_base32suffix"` |
| `encoding.TextMarshaler` / `TextUnmarshaler` | Same text form (JSON uses this automatically) |
| `driver.Valuer` | `UUID[P]` → UUID string, `Int64[P]` → `int64` |
| `sql.Scanner` | `UUID[P]` ← `string`/`[]byte`/`[16]byte`, `Int64[P]` ← `int64` |

## Int64 bit layout

```
[48-bit unix ms timestamp][15-bit crypto/rand] = 63 bits, always positive
```

Stored as Postgres `BIGINT`. Collision table: 10 IDs/sec → ~1 per 7,500 days; 100/sec → ~1 per 1.8 hours; 1,000/sec → ~1 per 65 seconds.

## Benchmarks

Apple M4 Pro, Go 1.26.1:

```
BenchmarkInt64_String         ~19 ns/op    24 B/op    1 allocs/op
BenchmarkInt64_MarshalText    ~18 ns/op    24 B/op    1 allocs/op
BenchmarkInt64_Parse          ~18 ns/op     0 B/op    0 allocs/op
BenchmarkUUID_String          ~24 ns/op    32 B/op    1 allocs/op
BenchmarkUUID_MarshalText     ~23 ns/op    32 B/op    1 allocs/op
BenchmarkUUID_Parse           ~33 ns/op     0 B/op    0 allocs/op
```

Parse is zero-allocation. Encode paths do a single allocation for the output buffer.

## License

Copyright (c) 2026 PT Services DMCC

Licensed under either:

- Apache License, Version 2.0, ([LICENSE-APACHE](./LICENSE-APACHE) or <http://www.apache.org/licenses/LICENSE-2.0>), or
- MIT license ([LICENSE-MIT](./LICENSE-MIT) or <http://opensource.org/licenses/MIT>)

as your option.

The SPDX license identifier for this project is `MIT` OR `Apache-2.0`.

## Contribution

Unless you explicitly state otherwise, any contribution intentionally submitted for inclusion in the work by you, as defined in the Apache-2.0 license, shall be dual licensed as above, without any additional terms or conditions.
