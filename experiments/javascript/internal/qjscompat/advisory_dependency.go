package qjs

// Keep the exact upstream module in the assessment import/scan inventory.
// govulncheck cannot associate copied source with its original module path;
// scanning the upstream copy conservatively retains its Go advisory coverage.
// This does not inspect either copy's embedded C/Wasm implementation.
import _ "github.com/fastschema/qjs"
