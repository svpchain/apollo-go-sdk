// Read-only example. APOLLO_RPC_URL must point to an Apollo Core HTTP JSON-RPC endpoint.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	apollo "github.com/svpchain/apollo-go-sdk"
)

func main() {
	endpoint := os.Getenv("APOLLO_RPC_URL")
	if endpoint == "" {
		log.Fatal("set APOLLO_RPC_URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	info, err := apollo.NewInfo(endpoint)
	if err != nil {
		log.Fatal(err)
	}
	chain, err := info.ChainInfo(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("chain", chain.ChainID, "genesis", chain.GenesisHash, "height", chain.ExecutedHeight)
	meta, err := info.Meta(ctx)
	if err != nil {
		log.Fatal(err)
	}
	for _, m := range meta.Markets {
		price, err := apollo.PriceUnitsToPrice(int64(m.Mark), m)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%d %s %s mark=%s\n", m.ID, m.Ticker, m.Status, price)
	}
}
