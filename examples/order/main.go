// Default: prepare/sign only. Passing -submit explicitly broadcasts the prepared order.
package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	apollo "github.com/svpchain/apollo-go-sdk"
)

func main() {
	account := flag.Uint("account", 0, "Core account ID (required)")
	slot := flag.Uint("slot", 1, "approved agent slot 1..4")
	coin := flag.String("coin", "", "exact market ticker (required)")
	price := flag.String("price", "", "limit price, decimal string (required)")
	size := flag.String("size", "", "base size, decimal string (required)")
	sell := flag.Bool("sell", false, "sell instead of buy")
	submit := flag.Bool("submit", false, "explicitly broadcast the signed order")
	flag.Parse()
	if *account == 0 || uint64(*account) > uint64(^uint32(0)) || *slot < 1 || *slot > 4 || *coin == "" || *price == "" || *size == "" {
		log.Fatal("set valid -account, -slot, -coin, -price and -size")
	}
	seed, err := hex.DecodeString(strings.TrimPrefix(os.Getenv("APOLLO_AGENT_SEED"), "0x"))
	if err != nil || len(seed) != 32 {
		log.Fatal("set APOLLO_AGENT_SEED to your own 32-byte approved seed")
	}
	genesis, err := apollo.ParseHash(os.Getenv("APOLLO_GENESIS_HASH"))
	if err != nil {
		log.Fatal("set APOLLO_GENESIS_HASH to your trusted genesis hash")
	}
	signer, err := apollo.NewAgentSigner(seed, uint8(*slot))
	if err != nil {
		log.Fatal(err)
	}
	exchange, err := apollo.NewExchange(os.Getenv("APOLLO_RPC_URL"), signer, apollo.WithAccount(uint32(*account)), apollo.WithExpectedGenesisHash(genesis))
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	prepared, err := exchange.PrepareOrder(ctx, apollo.CreateOrderRequest{Coin: *coin, IsBuy: !*sell, Price: *price, Size: *size, OrderType: apollo.OrderType{Limit: &apollo.LimitOrderType{TIF: apollo.Gtc}}})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("prepared hash", prepared.TransactionHash, "nonce", prepared.Nonce)
	if !*submit {
		fmt.Println("Not broadcast. Add -submit only when you intend to place the order.")
		return
	}
	response, err := exchange.SubmitPrepared(ctx, []apollo.PreparedAction{prepared})
	if err != nil {
		log.Fatal(err)
	}
	for _, status := range response.Statuses {
		fmt.Printf("submit=%s outcome=%s reject=%s\n", status.SubmitStatus, status.Outcome, status.Error)
	}
}
