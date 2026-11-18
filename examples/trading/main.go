// Command trading queries orders or explicitly prepares/submits one trading action.
// All account and signer defaults come from APOLLO_* environment variables.
package main

import (
	"context"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	apollo "github.com/svpchain/apollo-go-sdk"
)

type config struct {
	action, account, coin, orderID, price, size, slot string
	submit, sell                                      bool
}

func main() {
	if err := run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		var unknown *apollo.SubmissionUnknownError
		var tracking *apollo.TrackingError
		if errors.As(err, &unknown) {
			for _, p := range unknown.Prepared {
				fmt.Fprintf(os.Stderr, "Reconcile account=%d nonce=%d hash=%s\n", p.Account, p.Nonce, p.TransactionHash)
			}
		} else if errors.As(err, &tracking) {
			printResponse(os.Stderr, tracking.Response)
		}
		os.Exit(1)
	}
}

func run(args []string, env func(string) string, out, errOut io.Writer) error {
	var c config
	fs := flag.NewFlagSet("trading", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.StringVar(&c.action, "action", "orders", "orders (read only), order, cancel or modify")
	fs.StringVar(&c.account, "account", env("APOLLO_ACCOUNT"), "account ID (or APOLLO_ACCOUNT)")
	fs.StringVar(&c.coin, "coin", env("APOLLO_COIN"), "full market ticker (or APOLLO_COIN)")
	fs.StringVar(&c.orderID, "order-id", env("APOLLO_ORDER_ID"), "target order ID, digits without n (or APOLLO_ORDER_ID)")
	fs.StringVar(&c.price, "price", env("APOLLO_PRICE"), "decimal price (or APOLLO_PRICE)")
	fs.StringVar(&c.size, "size", env("APOLLO_SIZE"), "decimal base size (or APOLLO_SIZE)")
	fs.StringVar(&c.slot, "slot", env("APOLLO_AGENT_SLOT"), "approved agent slot, defaults to 1")
	fs.BoolVar(&c.sell, "sell", false, "place a sell order instead of a buy")
	fs.BoolVar(&c.submit, "submit", false, "explicitly broadcast order/cancel/modify; default is prepare only")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected positional argument")
	}
	switch c.action {
	case "orders", "order", "cancel", "modify":
	default:
		return errors.New("-action must be orders, order, cancel or modify")
	}
	if c.action == "orders" && c.submit {
		return errors.New("orders is read only; -submit applies to order/cancel/modify")
	}
	account, err := unsigned(c.account, 32, "account")
	if err != nil {
		return err
	}
	var orderID uint64
	if c.action == "cancel" || c.action == "modify" || (c.action == "orders" && c.orderID != "") {
		orderID, err = unsigned(c.orderID, 64, "order-id")
		if err != nil {
			return err
		}
	}
	if c.action != "orders" && strings.TrimSpace(c.coin) == "" {
		return errors.New("set APOLLO_COIN or pass -coin")
	}
	if (c.action == "order" || c.action == "modify") && (c.price == "" || c.size == "") {
		return errors.New("set APOLLO_PRICE/APOLLO_SIZE or pass -price/-size")
	}
	endpoint := strings.TrimSpace(env("APOLLO_RPC_URL"))
	if endpoint == "" {
		return errors.New("set APOLLO_RPC_URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if c.action == "orders" {
		info, err := apollo.NewInfo(endpoint)
		if err != nil {
			return err
		}
		if expected := strings.TrimSpace(env("APOLLO_GENESIS_HASH")); expected != "" {
			hash, err := apollo.ParseHash(expected)
			if err != nil {
				return errors.New("APOLLO_GENESIS_HASH must be a 32-byte hex hash")
			}
			chain, err := info.ChainInfo(ctx)
			if err != nil {
				return err
			}
			if chain.GenesisHash != hash {
				return &apollo.ChainIdentityError{Expected: hash, Actual: chain.GenesisHash}
			}
		}
		return printOrders(ctx, info, uint32(account), c.coin, orderID, out)
	}
	genesis, err := apollo.ParseHash(env("APOLLO_GENESIS_HASH"))
	if err != nil {
		return errors.New("set APOLLO_GENESIS_HASH to the trusted 32-byte hash")
	}
	seed, err := hex.DecodeString(strings.TrimPrefix(strings.TrimSpace(env("APOLLO_AGENT_SEED")), "0x"))
	if err != nil || len(seed) != 32 {
		return errors.New("set APOLLO_AGENT_SEED to a 32-byte approved seed in hex")
	}
	if c.slot == "" {
		c.slot = "1"
	}
	slot, err := unsigned(c.slot, 8, "slot")
	if err != nil || slot > 4 {
		return errors.New("slot must be 1..4")
	}
	signer, err := apollo.NewAgentSigner(seed, uint8(slot))
	if err != nil {
		return err
	}
	exchange, err := apollo.NewExchange(endpoint, signer, apollo.WithAccount(uint32(account)), apollo.WithExpectedGenesisHash(genesis))
	if err != nil {
		return err
	}
	var prepared apollo.PreparedAction
	switch c.action {
	case "order":
		prepared, err = exchange.PrepareOrder(ctx, apollo.CreateOrderRequest{Coin: c.coin, IsBuy: !c.sell, Price: c.price, Size: c.size})
	case "cancel", "modify":
		m, marketErr := exchange.Info().Market(ctx, c.coin)
		if marketErr != nil {
			return marketErr
		}
		if m.ID >= 65535 {
			return errors.New("market ID cannot be encoded")
		}
		action := apollo.Action{Type: apollo.Cancel, Account: uint32(account), Market: uint16(m.ID), OrderID: orderID}
		if c.action == "modify" {
			if m.Status != apollo.MarketActive {
				return errors.New("market is not active")
			}
			action.Type = apollo.Modify
			action.Price, err = apollo.PriceToSubticks(c.price, m)
			if err != nil {
				return err
			}
			action.Size, err = apollo.SizeToQuantums(c.size, m)
			if err != nil {
				return err
			}
		}
		prepared, err = exchange.Prepare(ctx, action)
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "prepared hash: %s nonce: %d\n", prepared.TransactionHash, prepared.Nonce)
	if !c.submit {
		fmt.Fprintln(out, "Not broadcast. Add -submit to execute this action.")
		return nil
	}
	response, err := exchange.SubmitPrepared(ctx, []apollo.PreparedAction{prepared})
	if err != nil {
		return err
	}
	printResponse(out, response)
	if !response.Successful() {
		return errors.New("action not confirmed successful; reconcile the original account/nonce before retrying")
	}
	if c.action == "cancel" {
		m, err := exchange.Info().Market(ctx, c.coin)
		if err != nil {
			return err
		}
		after, err := exchange.Info().OpenOrders(ctx, uint32(account), &m.ID)
		if err != nil {
			return err
		}
		stillOpen := false
		for _, order := range after.Orders {
			if uint64(order.OrderID) == orderID {
				stillOpen = true
			}
		}
		fmt.Fprintln(out, "Still open after cancellation:", stillOpen)
		if stillOpen {
			return errors.New("cancelled order still appears in open orders; reconcile its state")
		}
	}
	return nil
}

func unsigned(value string, bits int, name string) (uint64, error) {
	if value == "" {
		return 0, fmt.Errorf("set APOLLO_%s or pass -%s", strings.ToUpper(strings.ReplaceAll(name, "-", "_")), name)
	}
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("%s must contain decimal digits only, without n", name)
		}
	}
	n, err := strconv.ParseUint(value, 10, bits)
	if err != nil || n == 0 {
		return 0, fmt.Errorf("%s must be a nonzero uint%d", name, bits)
	}
	return n, nil
}

func printOrders(ctx context.Context, info *apollo.Info, account uint32, coin string, id uint64, out io.Writer) error {
	meta, err := info.Meta(ctx)
	if err != nil {
		return err
	}
	var marketID *uint32
	markets := make(map[uint32]apollo.Market)
	for _, m := range meta.Markets {
		markets[m.ID] = m
		if m.Ticker == coin {
			value := m.ID
			marketID = &value
		}
	}
	if coin != "" && marketID == nil {
		return fmt.Errorf("market not found: %s", coin)
	}
	result, err := info.OpenOrders(ctx, account, marketID)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "Account:", account, "height:", uint64(result.At.ExecutedHeight))
	fmt.Fprintln(out, "order_id\tcoin\tside\tprice\tremaining\tposition")
	found := 0
	for _, order := range result.Orders {
		if id != 0 && uint64(order.OrderID) != id {
			continue
		}
		m, ok := markets[order.Market]
		if !ok {
			return fmt.Errorf("missing metadata for market %d", order.Market)
		}
		price, err := apollo.SubticksToPrice(int64(order.Price), m)
		if err != nil {
			return err
		}
		size, err := apollo.QuantumsToSize(int64(order.Remaining), m)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "%d\t%s\t%s\t%s\t%s\t%s\n", order.OrderID, m.Ticker, order.Side, price, size, order.Position)
		found++
	}
	if found == 0 {
		fmt.Fprintln(out, "No matching open orders. Check execution and fills if the final order state is unknown.")
	}
	return nil
}

func printResponse(out io.Writer, response *apollo.OrderResponse) {
	if response == nil {
		return
	}
	for _, status := range response.Statuses {
		fmt.Fprintf(out, "submit=%s outcome=%s nonce=%d hash=%s error=%s\n", status.SubmitStatus, status.Outcome, status.Prepared.Nonce, status.Prepared.TransactionHash, status.Error)
		if status.Execution != nil {
			orderID := ""
			if status.Execution.OrderID != nil {
				orderID = strconv.FormatUint(uint64(*status.Execution.OrderID), 10)
			}
			fmt.Fprintf(out, "ok=%t order_id=%s fills=%d\n", status.Execution.OK, orderID, status.Execution.Fills)
		}
	}
}
