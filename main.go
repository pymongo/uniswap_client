package main

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

const (
	nodeURL     = "https://rpcapi.fantom.network"
	wsNodeURL   = "wss://wsapi.fantom.network/"
	pairABI     = `[{"constant":true,"inputs":[],"name":"getReserves","outputs":[{"name":"_reserve0","type":"uint112"},{"name":"_reserve1","type":"uint112"},{"name":"_blockTimestampLast","type":"uint32"}],"payable":false,"stateMutability":"view","type":"function"},{"anonymous":false,"inputs":[{"indexed":false,"name":"reserve0","type":"uint112"},{"indexed":false,"name":"reserve1","type":"uint112"}],"name":"Sync","type":"event"},{"anonymous":false,"inputs":[{"indexed":true,"name":"sender","type":"address"},{"indexed":false,"name":"amount0In","type":"uint256"},{"indexed":false,"name":"amount1In","type":"uint256"},{"indexed":false,"name":"amount0Out","type":"uint256"},{"indexed":false,"name":"amount1Out","type":"uint256"},{"indexed":true,"name":"to","type":"address"}],"name":"Swap","type":"event"}]`
	syncEvent   = "Sync"
	swapEvent   = "Swap"
)

var pairAddresses = []common.Address{
	common.HexToAddress("0xaC97153e7ce86fB3e61681b969698AF7C22b4B12"),
	common.HexToAddress("0x084F933B6401a72291246B5B5eD46218a68773e6"),
	common.HexToAddress("0x8dD580271D823CBDC4a1C6153f69Dad594C521Fd"),
	common.HexToAddress("0x2D0Ed226891E256d94F1071E2F94FBcDC9060E14"),
}

type Reserves struct {
	Reserve0 *big.Int
	Reserve1 *big.Int
}

var reserves = make(map[common.Address]Reserves)

func main() {
	// Initialize HTTP client
	client, err := ethclient.Dial(nodeURL)
	if err != nil {
		log.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}

	// Query initial reserves
	for _, addr := range pairAddresses {
		queryReserves(client, addr)
	}

	// Initialize WebSocket client
	wsClient, err := rpc.Dial(wsNodeURL)
	if err != nil {
		log.Fatalf("Failed to connect to the Ethereum websocket client: %v", err)
	}

	// Subscribe to Sync and Swap events
	for _, addr := range pairAddresses {
		go subscribeEvents(wsClient, addr)
	}

	// Prevent the main function from exiting
	select {}
}

func queryReserves(client *ethclient.Client, pairAddress common.Address) {
	contract, err := abi.JSON(strings.NewReader(pairABI))
	if err != nil {
		log.Fatalf("Failed to parse contract ABI: %v", err)
	}

	callData, err := contract.Pack("getReserves")
	if err != nil {
		log.Fatalf("Failed to pack call data: %v", err)
	}

	msg := ethereum.CallMsg{
		To:   &pairAddress,
		Data: callData,
	}

	res, err := client.CallContract(context.Background(), msg, nil)
	if err != nil {
		log.Fatalf("Failed to call contract: %v", err)
	}

	outputs, err := contract.Unpack("getReserves", res)
	if err != nil {
		log.Fatalf("Failed to unpack call result: %v", err)
	}

	reserves[pairAddress] = Reserves{
		Reserve0: outputs[0].(*big.Int),
		Reserve1: outputs[1].(*big.Int),
	}

	fmt.Printf("Initial reserves for %s: %v\n", pairAddress.Hex(), reserves[pairAddress])
}

func subscribeEvents(client *rpc.Client, pairAddress common.Address) {
	ethClient := ethclient.NewClient(client)
	contract, err := abi.JSON(strings.NewReader(pairABI))
	if err != nil {
		log.Fatalf("Failed to parse contract ABI: %v", err)
	}

	query := ethereum.FilterQuery{
		Addresses: []common.Address{pairAddress},
	}

	logs := make(chan types.Log)
	sub, err := ethClient.SubscribeFilterLogs(context.Background(), query, logs)
	if err != nil {
		log.Fatalf("Failed to subscribe to logs: %v", err)
	}

	for {
		select {
		case err := <-sub.Err():
			log.Fatalf("Subscription error: %v", err)
		case vLog := <-logs:
			handleLog(contract, vLog)
		}
	}
}

func handleLog(contract abi.ABI, vLog types.Log) {
	switch vLog.Topics[0].Hex() {
	case contract.Events[syncEvent].ID.Hex():
		var event struct {
			Reserve0 *big.Int
			Reserve1 *big.Int
		}
		err := contract.UnpackIntoInterface(&event, syncEvent, vLog.Data)
		if err != nil {
			log.Fatalf("Failed to unpack Sync event: %v", err)
		}
		pairAddress := vLog.Address
		reserves[pairAddress] = Reserves{
			Reserve0: event.Reserve0,
			Reserve1: event.Reserve1,
		}
		fmt.Printf("Updated reserves for %s: %v\n", pairAddress.Hex(), reserves[pairAddress])
	case contract.Events[swapEvent].ID.Hex():
		// Handle Swap event similarly if needed
	}
}
