package apollo

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Uint64 is a proto3 uint64, serialized as a decimal JSON string.
type Uint64 uint64

func (v Uint64) MarshalJSON() ([]byte, error) { return json.Marshal(strconv.FormatUint(uint64(v), 10)) }
func (v *Uint64) UnmarshalJSON(b []byte) error {
	s, err := integerText(b)
	if err != nil {
		return err
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return err
	}
	*v = Uint64(n)
	return nil
}

// Int64 is a proto3 int64, serialized as a decimal JSON string.
type Int64 int64

func (v Int64) MarshalJSON() ([]byte, error) { return json.Marshal(strconv.FormatInt(int64(v), 10)) }
func (v *Int64) UnmarshalJSON(b []byte) error {
	s, err := integerText(b)
	if err != nil {
		return err
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return err
	}
	*v = Int64(n)
	return nil
}
func integerText(b []byte) (string, error) {
	if bytes.Equal(b, []byte("null")) {
		return "", fmt.Errorf("integer cannot be null")
	}
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return "", err
		}
		return s, nil
	}
	return string(b), nil
}

// HexBytes uses Apollo's 0x JSON representation, not base64.
type HexBytes []byte

func (b HexBytes) String() string               { return "0x" + hex.EncodeToString(b) }
func (b HexBytes) MarshalJSON() ([]byte, error) { return json.Marshal(b.String()) }
func (b *HexBytes) UnmarshalJSON(v []byte) error {
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return err
	}
	raw, err := decodeHex(s, -1)
	if err != nil {
		return err
	}
	*b = raw
	return nil
}

type Hash [32]byte
type Address [20]byte

func (h Hash) String() string                  { return HexBytes(h[:]).String() }
func (a Address) String() string               { return HexBytes(a[:]).String() }
func (h Hash) MarshalJSON() ([]byte, error)    { return json.Marshal(h.String()) }
func (a Address) MarshalJSON() ([]byte, error) { return json.Marshal(a.String()) }
func (h *Hash) UnmarshalJSON(v []byte) error {
	var s string
	if e := json.Unmarshal(v, &s); e != nil {
		return e
	}
	x, e := ParseHash(s)
	if e == nil {
		*h = x
	}
	return e
}
func (a *Address) UnmarshalJSON(v []byte) error {
	var s string
	if e := json.Unmarshal(v, &s); e != nil {
		return e
	}
	x, e := ParseAddress(s)
	if e == nil {
		*a = x
	}
	return e
}
func ParseHash(s string) (Hash, error) {
	var out Hash
	b, e := decodeHex(s, 32)
	if e == nil {
		copy(out[:], b)
	}
	return out, e
}
func ParseAddress(s string) (Address, error) {
	var out Address
	b, e := decodeHex(s, 20)
	if e == nil {
		copy(out[:], b)
	}
	return out, e
}
func decodeHex(s string, size int) ([]byte, error) {
	if !strings.HasPrefix(s, "0x") {
		return nil, fmt.Errorf("hex value requires 0x prefix")
	}
	b, e := hex.DecodeString(s[2:])
	if e != nil {
		return nil, e
	}
	if size >= 0 && len(b) != size {
		return nil, fmt.Errorf("expected %d bytes, got %d", size, len(b))
	}
	return b, nil
}
