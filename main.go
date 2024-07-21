package main

import (
	"context"
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
	nodeURL      = "https://rpcapi.fantom.network"
	wsNodeURL    = "wss://wsapi.fantom.network/"
	pairABI      = `[{"constant":true,"inputs":[],"name":"getReserves","outputs":[{"name":"_reserve0","type":"uint112"},{"name":"_reserve1","type":"uint112"},{"name":"_blockTimestampLast","type":"uint32"}],"payable":false,"stateMutability":"view","type":"function"},{"anonymous":false,"inputs":[{"indexed":false,"name":"reserve0","type":"uint112"},{"indexed":false,"name":"reserve1","type":"uint112"}],"name":"Sync","type":"event"},{"anonymous":false,"inputs":[{"indexed":true,"name":"sender","type":"address"},{"indexed":false,"name":"amount0In","type":"uint256"},{"indexed":false,"name":"amount1In","type":"uint256"},{"indexed":false,"name":"amount0Out","type":"uint256"},{"indexed":false,"name":"amount1Out","type":"uint256"},{"indexed":true,"name":"to","type":"address"}],"name":"Swap","type":"event"}]`
	syncEventStr = "Sync"
	swapEventStr = "Swap"
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

var reserves = make(map[common.Address]Reserves, len(pairAddresses))

func main() {
	log.SetFlags(log.Lmicroseconds | log.Lshortfile)

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
	subscribeEvents(wsClient, pairAddresses)
	panic("unreachable")
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

	log.Printf("Initial reserves for %s: %v\n", pairAddress.Hex(), reserves[pairAddress])
}

func subscribeEvents(client *rpc.Client, pairAddresses []common.Address) {
	ethClient := ethclient.NewClient(client)
	contract, err := abi.JSON(strings.NewReader(pairABI))
	if err != nil {
		log.Fatalf("Failed to parse contract ABI: %v", err)
	}

	query := ethereum.FilterQuery{
		Addresses: pairAddresses,
		// This filters for the topics related to UniswapV2Pair Sync and Swap events
		Topics: [][]common.Hash{
			{contract.Events[syncEventStr].ID, contract.Events[swapEventStr].ID},
		},
	}

	logs := make(chan types.Log)
	sub, err := ethClient.SubscribeFilterLogs(context.Background(), query, logs)
	if err != nil {
		log.Fatalf("Failed to subscribe to logs: %v", err)
	}
	defer sub.Unsubscribe() // Ensure we unsubscribe when done

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
	// Explicitly log the event type for debugging purposes
	log.Printf("Received log: %+v\n", vLog)
	switch vLog.Topics[0].Hex() {
	case contract.Events[syncEventStr].ID.Hex():
		var syncEvent struct {
			Reserve0 *big.Int
			Reserve1 *big.Int
		}
		err := contract.UnpackIntoInterface(&syncEvent, syncEventStr, vLog.Data)
		if err != nil {
			log.Fatalf("Failed to unpack Sync event: %v", err)
		}
		pairAddress := vLog.Address
		reserves[pairAddress] = Reserves{
			Reserve0: syncEvent.Reserve0,
			Reserve1: syncEvent.Reserve1,
		}
		log.Printf("Updated reserves for %s: %v\n", pairAddress.Hex(), reserves[pairAddress])
	case contract.Events[swapEventStr].ID.Hex():
		var swapEvent struct {
			Sender     common.Address
			Amount0In  *big.Int
			Amount1In  *big.Int
			Amount0Out *big.Int
			Amount1Out *big.Int
			To         common.Address
		}
		err := contract.UnpackIntoInterface(&swapEvent, swapEventStr, vLog.Data)
		if err != nil {
			log.Printf("Failed to unpack Swap event: %v", err)
			return
		}

		pairAddress := vLog.Address
		currentReserves, exists := reserves[pairAddress]
		if !exists {
			log.Printf("No reserves found for address %s", pairAddress.Hex())
			return
		}

		// Update reserves based on swap event
		updatedReserves := Reserves{
			Reserve0: new(big.Int).Set(currentReserves.Reserve0),
			Reserve1: new(big.Int).Set(currentReserves.Reserve1),
		}

		updatedReserves.Reserve0.Sub(updatedReserves.Reserve0, swapEvent.Amount0Out)
		updatedReserves.Reserve0.Add(updatedReserves.Reserve0, swapEvent.Amount0In)

		updatedReserves.Reserve1.Sub(updatedReserves.Reserve1, swapEvent.Amount1Out)
		updatedReserves.Reserve1.Add(updatedReserves.Reserve1, swapEvent.Amount1In)

		reserves[pairAddress] = updatedReserves
		log.Printf("Updated reserves for %s after swap: %v", pairAddress.Hex(), reserves[pairAddress])
	}
}
