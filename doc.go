// Package apollo provides Apollo Core trading over HTTP JSON-RPC.
//
// Info contains read-only methods; Exchange constructs, signs and submits native
// Apollo actions. Prices and quantities in order requests are decimal strings.
// RPC int64/uint64 fields are represented by Go integers with lossless JSON codecs.
// Submit acceptance, execution and actual fills are distinct states.
package apollo
