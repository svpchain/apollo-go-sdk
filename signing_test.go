package apollo

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

type vectors struct {
	Genesis string `json:"genesis_hash"`
	Keys    struct {
		AgentSeed    string `json:"agent_seed"`
		AgentPublic  string `json:"agent_public"`
		OwnerSecret  string `json:"owner_secret"`
		OwnerAddress string `json:"owner_address"`
	} `json:"keys"`
	Actions []struct {
		Name    string `json:"name"`
		Action  Action `json:"action"`
		Encoded string `json:"encoded"`
		Nonce   Uint64 `json:"nonce"`
		Owner   struct {
			Auth   string `json:"auth"`
			Digest string `json:"digest"`
		} `json:"owner"`
		Agent *struct {
			Slot    uint8  `json:"slot"`
			Seq     uint32 `json:"seq"`
			Auth    string `json:"auth"`
			Message string `json:"message"`
		} `json:"agent"`
	} `json:"actions"`
}

func loadVectors(t *testing.T) vectors {
	t.Helper()
	data, e := os.ReadFile("internal/testdata/actions.json")
	if e != nil {
		t.Fatal(e)
	}
	var v vectors
	if e = json.Unmarshal(data, &v); e != nil {
		t.Fatal(e)
	}
	return v
}
func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, e := hex.DecodeString(s)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestChainVectors(t *testing.T) {
	v := loadVectors(t)
	genesis, err := ParseHash("0x" + v.Genesis)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := NewOwnerSigner(unhex(t, v.Keys.OwnerSecret))
	if err != nil {
		t.Fatal(err)
	}
	if owner.Identity().Owner.String() != "0x"+v.Keys.OwnerAddress {
		t.Fatal("owner public address mismatch")
	}
	covered := 0
	for _, test := range v.Actions {
		t.Run(test.Name, func(t *testing.T) {
			if !isTradingAction(test.Action.Type) {
				t.Skip("outside trading codec scope")
			}
			covered++
			action, err := EncodeAction(test.Action)
			if err != nil {
				t.Fatal(err)
			}
			if hex.EncodeToString(action[:]) != test.Encoded {
				t.Fatalf("action bytes mismatch: %x != %s", action, test.Encoded)
			}
			digest := OwnerDigest(genesis, action, uint64(test.Nonce))
			if hex.EncodeToString(digest[:]) != test.Owner.Digest {
				t.Fatal("owner digest mismatch")
			}
			expected := unhex(t, test.Owner.Auth)
			a, err := PackOwnerAuth(uint64(test.Nonce), expected[13:])
			if err != nil {
				t.Fatal(err)
			}
			raw, err := EncodeAuth(a)
			if err != nil || !bytes.Equal(raw[:], expected) {
				t.Fatal("owner auth mismatch", err)
			}
			recovered, err := RecoverOwner(digest, a.Signature)
			if err != nil || recovered != owner.Identity().Owner {
				t.Fatal("owner recovery mismatch", err)
			}
			// RFC6979 signatures must match the protocol vectors.
			own, err := owner.Sign(context.Background(), SignRequest{GenesisHash: genesis, Action: action, Nonce: uint64(test.Nonce)})
			if err != nil {
				t.Fatal(err)
			}
			packed, err := EncodeAuth(own)
			if err != nil || !bytes.Equal(packed[:], expected) {
				t.Fatal("local owner signature mismatch", err)
			}
			if test.Agent != nil {
				agent, err := NewAgentSigner(unhex(t, v.Keys.AgentSeed), test.Agent.Slot)
				if err != nil {
					t.Fatal(err)
				}
				if agent.Identity().PublicKey == ([32]byte{}) {
					t.Fatal("empty key")
				}
				msg, err := AgentMessage(genesis, action, uint64(test.Nonce), test.Agent.Slot, test.Agent.Seq)
				if err != nil || hex.EncodeToString(msg[:]) != test.Agent.Message {
					t.Fatal("agent message mismatch", err)
				}
				signed, err := agent.Sign(context.Background(), SignRequest{genesis, action, uint64(test.Nonce), test.Agent.Seq})
				if err != nil {
					t.Fatal(err)
				}
				packed, err := EncodeAuth(signed)
				if err != nil || hex.EncodeToString(packed[:]) != test.Agent.Auth {
					t.Fatal("agent auth mismatch", err)
				}
				if err = verifySignature(agent.Identity(), SignRequest{genesis, action, uint64(test.Nonce), test.Agent.Seq}, signed); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	if covered != 18 {
		t.Fatalf("insufficient vector coverage: %d", covered)
	}
}
func TestCanonicalTransactionHash(t *testing.T) {
	var action [48]byte
	action[0] = 1
	got := TransactionHash(action, [78]byte{})
	if got.String() != "0xfd74664e66f91117552e2b7760281b16a856867321c2d12ee312d887be522a3f" {
		t.Fatal(got)
	}
}
func TestSignerInputAndCopy(t *testing.T) {
	seed := bytes.Repeat([]byte{7}, 32)
	agent, e := NewAgentSigner(seed, 1)
	if e != nil {
		t.Fatal(e)
	}
	copyID := agent.Identity()
	seed[0] = 1
	if agent.Identity() != copyID {
		t.Fatal("caller mutated signer")
	}
	for _, key := range [][]byte{nil, make([]byte, 32), bytes.Repeat([]byte{255}, 32)} {
		if _, e := NewOwnerSigner(key); e == nil {
			t.Fatal("accepted invalid owner key")
		}
	}
	for _, slot := range []uint8{0, 5, 255} {
		if _, e := NewAgentSigner(seed, slot); e == nil {
			t.Fatal("accepted invalid slot")
		}
	}
	v := loadVectors(t)
	entry := v.Actions[3]
	var raw [65]byte
	copy(raw[:], unhex(t, entry.Owner.Auth)[13:])
	raw[64] = 2
	if _, e := RecoverOwner(Hash{}, raw); e == nil {
		t.Fatal("accepted recovery id")
	}
	raw[64] = 27
	for i := 32; i < 64; i++ {
		raw[i] = 255
	}
	if _, e := RecoverOwner(Hash{}, raw); e == nil {
		t.Fatal("accepted high-s")
	}
}
