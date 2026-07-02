// Package importv2 defines the contract of the streaming import engine: the
// object/stream model, the converter interface, the issue/severity model and
// the request/result types. See docs/ImportV2Design.md.
//
// The package holds only data types and interfaces shared between the engine
// subpackages (engine, identity, resolve, persist, source, converters,
// adapter). Subpackages import this root package and never each other's
// internals; consumer-side seams (store, space, uploader, ...) are defined in
// the subpackage that consumes them.
package importv2
