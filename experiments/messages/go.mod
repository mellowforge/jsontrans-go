module github.com/mellowforge/jsontrans-go/experiments/messages

go 1.26.0

toolchain go1.26.10

require (
	github.com/mellowforge/jsontrans-go v1.1.1-0.20261103173822-e9b9c0c71fe0
	github.com/mellowforge/jsontrans-go/javascript v1.0.0
)

require (
	github.com/dlclark/regexp2/v2 v2.12.0 // indirect
	github.com/tetratelabs/wazero v1.12.0 // indirect
	golang.org/x/sys v0.44.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

replace github.com/mellowforge/jsontrans-go => ../..

replace github.com/mellowforge/jsontrans-go/javascript => ../../javascript
