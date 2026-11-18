package apollo

import (
	"context"
	"crypto/ed25519"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"

	secp "github.com/decred/dcrd/dcrec/secp256k1/v4"
	secpecdsa "github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"golang.org/x/crypto/sha3"
)

const (
	ActionBytes = 48
	AuthBytes   = 78
)

type Auth struct {
	Nonce     uint64
	Key       uint8
	Seq       uint32
	Signature [65]byte
}

func EncodeAuth(a Auth) ([78]byte, error) {
	var b [78]byte
	if a.Nonce == 0 || a.Key > 4 {
		return b, errors.New("invalid auth nonce or key")
	}
	if a.Key == 0 && a.Seq != 0 {
		return b, errors.New("owner auth seq must be zero")
	}
	if a.Key > 0 && a.Signature[64] != 0 {
		return b, errors.New("agent signature padding must be zero")
	}
	binary.LittleEndian.PutUint64(b[:8], a.Nonce)
	b[8] = a.Key
	binary.LittleEndian.PutUint32(b[9:13], a.Seq)
	copy(b[13:], a.Signature[:])
	return b, nil
}
func DecodeAuth(b [78]byte) (Auth, error) {
	a := Auth{Nonce: binary.LittleEndian.Uint64(b[:8]), Key: b[8], Seq: binary.LittleEndian.Uint32(b[9:13])}
	copy(a.Signature[:], b[13:])
	_, e := EncodeAuth(a)
	return a, e
}
func keccak(parts ...[]byte) Hash {
	h := sha3.NewLegacyKeccak256()
	for _, p := range parts {
		h.Write(p)
	}
	var out Hash
	copy(out[:], h.Sum(nil))
	return out
}

// TransactionHash hashes canonical bytes; it does not prove signature validity.
func TransactionHash(action [48]byte, auth [78]byte) Hash {
	return keccak([]byte("apollo-transaction-v1"), action[:], auth[:])
}
func AgentMessage(genesis Hash, action [48]byte, nonce uint64, slot uint8, seq uint32) ([109]byte, error) {
	var b [109]byte
	if nonce == 0 || slot < 1 || slot > 4 {
		return b, errors.New("agent requires nonzero nonce and slot 1..4")
	}
	copy(b[:16], "apollo-action-v3")
	copy(b[16:48], genesis[:])
	copy(b[48:96], action[:])
	binary.LittleEndian.PutUint64(b[96:104], nonce)
	b[104] = slot
	binary.LittleEndian.PutUint32(b[105:], seq)
	return b, nil
}
func word(n uint64) [32]byte { var w [32]byte; binary.BigEndian.PutUint64(w[24:], n); return w }

// OwnerDigest is the EIP-712 digest of Apollo/1, salt=genesis, Action(bytes action,uint64 nonce).
func OwnerDigest(genesis Hash, action [48]byte, nonce uint64) Hash {
	domainType := keccak([]byte("EIP712Domain(string name,string version,bytes32 salt)"))
	name := keccak([]byte("Apollo"))
	version := keccak([]byte("1"))
	domain := keccak(domainType[:], name[:], version[:], genesis[:])
	actionType := keccak([]byte("Action(bytes action,uint64 nonce)"))
	body := keccak(action[:])
	n := word(nonce)
	structure := keccak(actionType[:], body[:], n[:])
	return keccak([]byte{0x19, 0x01}, domain[:], structure[:])
}

type TypedDataField struct {
	Name string `json:"name"`
	Type string `json:"type"`
}
type TypedData struct {
	Types       map[string][]TypedDataField `json:"types"`
	PrimaryType string                      `json:"primaryType"`
	Domain      TypedDataDomain             `json:"domain"`
	Message     TypedDataMessage            `json:"message"`
}
type TypedDataDomain struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Salt    Hash   `json:"salt"`
}
type TypedDataMessage struct {
	Action HexBytes `json:"action"`
	Nonce  Uint64   `json:"nonce"`
}

func OwnerTypedData(genesis Hash, action [48]byte, nonce uint64) TypedData {
	return TypedData{Types: map[string][]TypedDataField{"EIP712Domain": {{"name", "string"}, {"version", "string"}, {"salt", "bytes32"}}, "Action": {{"action", "bytes"}, {"nonce", "uint64"}}}, PrimaryType: "Action", Domain: TypedDataDomain{"Apollo", "1", genesis}, Message: TypedDataMessage{Action: append(HexBytes(nil), action[:]...), Nonce: Uint64(nonce)}}
}
func publicAddress(key *secp.PublicKey) Address {
	raw := key.SerializeUncompressed()
	hash := keccak(raw[1:])
	var out Address
	copy(out[:], hash[12:])
	return out
}

// RecoverOwner rejects malformed scalars, invalid recovery bits and high-S signatures.
func RecoverOwner(digest Hash, signature [65]byte) (Address, error) {
	var zero Address
	rec := signature[64]
	if rec >= 27 {
		rec -= 27
	}
	if rec > 1 {
		return zero, errors.New("owner recovery bit must be 0/1 or 27/28")
	}
	r, s := new(big.Int).SetBytes(signature[:32]), new(big.Int).SetBytes(signature[32:64])
	order := secp.Params().N
	half := new(big.Int).Rsh(new(big.Int).Set(order), 1)
	if r.Sign() <= 0 || r.Cmp(order) >= 0 || s.Sign() <= 0 || s.Cmp(half) > 0 {
		return zero, errors.New("owner signature scalars invalid or high-S")
	}
	compact := make([]byte, 65)
	compact[0] = 27 + rec
	copy(compact[1:], signature[:64])
	key, _, err := secpecdsa.RecoverCompact(compact, digest[:])
	if err != nil {
		return zero, err
	}
	return publicAddress(key), nil
}
func PackOwnerAuth(nonce uint64, signature []byte) (Auth, error) {
	var a Auth
	if nonce == 0 || len(signature) != 65 {
		return a, errors.New("owner requires nonzero nonce and 65-byte signature")
	}
	a.Nonce = nonce
	copy(a.Signature[:], signature)
	return a, nil
}

type SignerIdentity struct {
	Slot      uint8
	Owner     Address
	PublicKey [32]byte
}
type SignRequest struct {
	GenesisHash Hash
	Action      [48]byte
	Nonce       uint64
	Seq         uint32
}

// Signer can be implemented by a local owner, a wallet callback or an Ed25519 agent.
// Exchange verifies returned signatures before making signed bytes available.
type Signer interface {
	Identity() SignerIdentity
	Sign(context.Context, SignRequest) (Auth, error)
}
type ownerSigner struct {
	key     *secp.PrivateKey
	address Address
}

func NewOwnerSigner(privateKey []byte) (Signer, error) {
	if len(privateKey) != 32 {
		return nil, errors.New("owner private key must be 32 bytes")
	}
	n := new(big.Int).SetBytes(privateKey)
	if n.Sign() <= 0 || n.Cmp(secp.Params().N) >= 0 {
		return nil, errors.New("owner private key is outside curve order")
	}
	key := secp.PrivKeyFromBytes(privateKey)
	return &ownerSigner{key: key, address: publicAddress(key.PubKey())}, nil
}
func (s *ownerSigner) Identity() SignerIdentity { return SignerIdentity{Owner: s.address} }
func (s *ownerSigner) String() string           { return "OwnerSigner(" + s.address.String() + ")" }
func (s *ownerSigner) Sign(ctx context.Context, r SignRequest) (Auth, error) {
	if e := ctx.Err(); e != nil {
		return Auth{}, e
	}
	h := OwnerDigest(r.GenesisHash, r.Action, r.Nonce)
	sig := secpecdsa.SignCompact(s.key, h[:], false)
	if sig[0] < 27 || sig[0] > 28 {
		return Auth{}, errors.New("unrepresentable recovery id")
	}
	var signature [65]byte
	copy(signature[:64], sig[1:])
	signature[64] = sig[0]
	return Auth{Nonce: r.Nonce, Signature: signature}, nil
}

type WalletSignFunc func(context.Context, TypedData) ([]byte, error)
type walletSigner struct {
	address Address
	sign    WalletSignFunc
}

func NewWalletSigner(owner Address, sign WalletSignFunc) (Signer, error) {
	if owner == (Address{}) || sign == nil {
		return nil, errors.New("wallet owner and callback are required")
	}
	return &walletSigner{owner, sign}, nil
}
func (s *walletSigner) Identity() SignerIdentity { return SignerIdentity{Owner: s.address} }
func (s *walletSigner) Sign(ctx context.Context, r SignRequest) (Auth, error) {
	if e := ctx.Err(); e != nil {
		return Auth{}, e
	}
	sig, e := s.sign(ctx, OwnerTypedData(r.GenesisHash, r.Action, r.Nonce))
	if e != nil {
		return Auth{}, e
	}
	return PackOwnerAuth(r.Nonce, sig)
}

type agentSigner struct {
	key      ed25519.PrivateKey
	identity SignerIdentity
}

func NewAgentSigner(seed []byte, slot uint8) (Signer, error) {
	if len(seed) != 32 || slot < 1 || slot > 4 {
		return nil, errors.New("agent requires 32-byte seed and slot 1..4")
	}
	key := ed25519.NewKeyFromSeed(append([]byte(nil), seed...))
	id := SignerIdentity{Slot: slot}
	copy(id.PublicKey[:], key.Public().(ed25519.PublicKey))
	return &agentSigner{key: key, identity: id}, nil
}
func (s *agentSigner) Identity() SignerIdentity { return s.identity }
func (s *agentSigner) String() string {
	return fmt.Sprintf("AgentSigner(slot=%d, publicKey=%s)", s.identity.Slot, hex.EncodeToString(s.identity.PublicKey[:]))
}
func (s *agentSigner) Sign(ctx context.Context, r SignRequest) (Auth, error) {
	if e := ctx.Err(); e != nil {
		return Auth{}, e
	}
	message, e := AgentMessage(r.GenesisHash, r.Action, r.Nonce, s.identity.Slot, r.Seq)
	if e != nil {
		return Auth{}, e
	}
	a := Auth{Nonce: r.Nonce, Key: s.identity.Slot, Seq: r.Seq}
	copy(a.Signature[:64], ed25519.Sign(s.key, message[:]))
	return a, nil
}
func verifySignature(identity SignerIdentity, r SignRequest, a Auth) error {
	if a.Nonce != r.Nonce || a.Key != identity.Slot {
		return errors.New("signer returned mismatching nonce or slot")
	}
	if a.Key == 0 {
		if a.Seq != 0 {
			return errors.New("owner signature seq must be zero")
		}
		address, e := RecoverOwner(OwnerDigest(r.GenesisHash, r.Action, r.Nonce), a.Signature)
		if e != nil {
			return e
		}
		if address != identity.Owner {
			return errors.New("signature does not belong to account owner")
		}
		return nil
	}
	if a.Seq != r.Seq || a.Signature[64] != 0 {
		return errors.New("agent auth seq or padding mismatch")
	}
	message, e := AgentMessage(r.GenesisHash, r.Action, r.Nonce, a.Key, a.Seq)
	if e != nil {
		return e
	}
	if !ed25519.Verify(identity.PublicKey[:], message[:], a.Signature[:64]) {
		return errors.New("invalid agent signature")
	}
	return nil
}
