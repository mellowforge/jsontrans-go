module github.com/mellowforge/jsontrans-go/experiments/javascript

go 1.26.0

toolchain go1.26.10

require (
	github.com/dop251/goja v0.0.0-20261102223426-a177d211bf60
	github.com/fastschema/qjs v0.0.6
	github.com/mellowforge/jsontrans-go v0.0.0
	github.com/tetratelabs/wazero v1.9.0
)

require (
	github.com/dlclark/regexp2/v2 v2.12.0 // indirect
	github.com/go-sourcemap/sourcemap v2.1.3+incompatible // indirect
	github.com/google/pprof v0.0.0-20230207041349-798e818bf904 // indirect
	golang.org/x/text v0.43.0 // indirect
)

replace github.com/mellowforge/jsontrans-go => ../..
