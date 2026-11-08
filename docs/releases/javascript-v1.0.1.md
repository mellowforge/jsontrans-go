# JavaScript processor v1.0.1

Module: `github.com/mellowforge/jsontrans-go/javascript`; repository tag:
`javascript/v1.0.1`. Go **1.26 or newer** remains required.

This patch replaces the minimum-core pseudo-version
`v1.1.1-0.20261103173822-e9b9c0c71fe0` with the published stable core **v1.2.0**.
The JavaScript API, guest, bridge, Wazero version and execution profile are
unchanged. Native JSONPath/JSONata remain independent of the JavaScript module.

## Install

After the `javascript/v1.0.1` tag is published:

```sh
go get github.com/mellowforge/jsontrans-go/javascript@v1.0.1
```

Go selects core v1.2.0 or a higher version required by the application. No local
`replace`, workspace or private-repository configuration is needed. The
repository's development replacement is ignored when consumed as a dependency.

Until patch publication, the already published v1.0.0 adapter also works with
core v1.2.0 when both are selected explicitly:

```sh
go get github.com/mellowforge/jsontrans-go/javascript@v1.0.0 \
  github.com/mellowforge/jsontrans-go@v1.2.0
```

## What this changes

- New installations of v1.0.1 resolve a stable core tag instead of the older
  pseudo-version. Existing application imports and JavaScript expressions do
  not need changes.
- The declared minimum includes `jsontrans.EvalPreparedJSON` and
  `Batch.EvalPreparedJSON`. These root helpers work through the same detached
  input conversion and fresh-guest execution as existing JavaScript calls.
- Packaging checks now exercise both prepared JSON helpers from the actual
  declared minimum in a separate consumer, without workspace replacements.
  Detached-checkout regressions cover both the stable tag and the historical
  pseudo-version.

The [core v1.2.0 notes](v1.2.0.md) describe its additive JSON-result APIs,
adapter-limit forwarding and dependency update. This patch does not change
the root module or introduce another processor.

## Preserved execution profile

The [production-approved synchronous profile](../javascript-execution-contract.md)
and [platform policy](../javascript-production-readiness.md) remain in effect:
independent guest state per evaluation, detached JSON values, missing/null
distinction, bounded conversion and cancellation, terminal guest OOM/InternalError,
and explicit engine ownership/Close. Async execution, imports, Node APIs, host
functions, filesystem/network access and runtime pooling remain excluded.

QuickJS-ng v0.18.0, the owned terminal-OOM patch, Wazero v1.12.0 and guest ABI 3
are unchanged. The embedded guest remains 1,034,164 bytes with SHA-256
`31aaf1d757a9071dafae75c424c49fb91d0ba70c455d5bde37afd37229b38dee`.
Guest caps still do not bound total process RSS.

The original [v1.0.0 release and acceptance record](javascript-v1.0.0.md)
retains its original dependency requirement, verification evidence and approval
date; this patch does not relabel those historical runs as new checks.
