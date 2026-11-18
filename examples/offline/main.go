// A complete offline signing example; no network access and no fixed private key.
package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"

	apollo "github.com/svpchain/apollo-go-sdk"
)

func main() {
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		log.Fatal(err)
	}
	signer, err := apollo.NewAgentSigner(seed, 1)
	if err != nil {
		log.Fatal(err)
	}
	action, err := apollo.EncodeAction(apollo.Action{Type: apollo.CancelAll, Account: 7, Market: 65535})
	if err != nil {
		log.Fatal(err)
	}
	auth, err := signer.Sign(context.Background(), apollo.SignRequest{GenesisHash: apollo.Hash{}, Action: action, Nonce: 1, Seq: 0})
	if err != nil {
		log.Fatal(err)
	}
	packed, err := apollo.EncodeAuth(auth)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("action bytes:", len(action), "auth bytes:", len(packed))
	fmt.Println("offline transaction hash:", apollo.TransactionHash(action, packed))
}
