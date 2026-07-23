# priompt proto

**The shared contracts and engines — the single source of truth.**
This repo is the one place shared code lives; no other repo carries a copy
that can drift:

- `proto/priompt/v1/prompt.proto` — the `PromptService` gRPC API the server
  implements and every client speaks (+ generated Go stubs in `gen/`).
- `claims/` — the JWT claims contract between the priompt-auth issuer and the
  core server's verifier.
- `validate/` — prompt well-formedness: the one definition of "valid",
  enforced by the server on write and serve, and by promptctl on commit.
- `semdiff/` — the semantic propagation diff engine, used by the server
  (publish/diff RPCs) and promptctl (local diff) alike.

| Consumer | How it uses this repo |
| --- | --- |
| **priompt** (server) | imports the generated Go stubs: `priomptproto/gen/priompt/v1` |
| **cli** (`promptctl`) | same Go stubs |
| **python-sdk** | its `buf.gen.yaml` reads `../proto/proto` and generates `priompt/v1/prompt_pb2*.py` in-package |
| **js-sdk** | vendors the `.proto` file at publish time (`npm run sync-proto`) and loads it at runtime — no codegen |
| **auth** (issuer) | imports `priomptproto/claims` — the same Claims type the server's verifier uses |

## Regenerating

Requires [buf](https://buf.build/docs/installation) — remote plugins, no protoc:

```sh
buf generate        # writes gen/priompt/v1/*.pb.go (committed)
```

Everything under `gen/` is generated — never hand-edit. After changing the
`.proto`, regenerate here, run `buf generate` in python-sdk, and
`npm run sync-proto` in js-sdk.

The Go module is `priomptproto`; sibling repos point at it with a `replace`
directive until it is published under its public path.
