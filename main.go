package main

import (
	"context"
	_ "embed"
	"log"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

//go:embed IUniswapV2Pair.abi.json
var pairAbiStr string

const (
	nodeURL      = "https://rpcapi.fantom.network"
	wsNodeURL    = "wss://wsapi.fantom.network/"
	syncEventStr = "Sync"
	swapEventStr = "Swap"
)

type Pair struct {
	addr       common.Address
	token0Addr common.Address
	token1Addr common.Address
	// e.g. 1e18
	decimalsMul0 *big.Int
	decimalsMul1 *big.Int
	reserve      Reserves
	// e.g. quote_coin/token1 is USDC so price is reserve0/reserve1, Vice versa
	quoteIsStableCoin bool
}

func (pair *Pair) amount0() float64 {
	reserve := new(big.Int).Set(pair.reserve.Reserve0)
	reserve.Div(reserve, pair.decimalsMul0)
	amount := new(big.Float).SetInt(reserve)
	float, _ := amount.Float64()
	return float
}
func (pair *Pair) amount1() float64 {
	reserve := new(big.Int).Set(pair.reserve.Reserve1)
	reserve.Div(reserve, pair.decimalsMul1)
	amount := new(big.Float).SetInt(reserve)
	float, _ := amount.Float64()
	return float
}

// WETH/USDC 2424683984456539796875385 1301258450287
/*
price 用 big.Int 换算 decimals, priceF 用 big.Float 换算 decimals 二者几乎没有误差

price= 3.820039 amount0= 83231.000000 amount1= 21788.000000
priceF=3.820073 amountF0=83231.921203 amountF1=21788.047366

price= 0.520731 amount0= 1271582.000000 amount1= 2441917.000000
priceF=0.520731 amountF0=1271582.547983 amountF1=2441917.863439

price= 0.520929 amount0= 2461380.000000 amount1= 1282203.000000
priceF=0.520928 amountF0=2461380.624467 amountF1=1282203.123785

price= 0.520714 amount0= 2637122.000000 amount1= 1373186.000000
priceF=0.520714 amountF0=2637122.261482 amountF1=1373186.008633
*/
func (pair *Pair) price() float64 {
	amount0 := pair.amount0()
	amount1 := pair.amount1()

	if pair.quoteIsStableCoin {
		return amount1 / amount0
	} else {
		return amount0 / amount1
	}
}
func (pair *Pair) amountFloat0() float64 {
	reserve := new(big.Float).SetInt(pair.reserve.Reserve0)
	amount := new(big.Float).Quo(reserve, new(big.Float).SetInt(pair.decimalsMul0))
	float, _ := amount.Float64()
	return float
}
func (pair *Pair) amountFloat1() float64 {
	reserve := new(big.Float).SetInt(pair.reserve.Reserve1)
	amount := new(big.Float).Quo(reserve, new(big.Float).SetInt(pair.decimalsMul1))
	float, _ := amount.Float64()
	return float
}
func (pair *Pair) priceFloat() float64 {
	amount0 := pair.amountFloat0()
	amount1 := pair.amountFloat1()

	if pair.quoteIsStableCoin {
		return amount1 / amount0
	} else {
		return amount0 / amount1
	}
}

type Reserves struct {
	Reserve0           *big.Int
	Reserve1           *big.Int
	BlockTimestampLast uint32
}

var pairAddresses = []common.Address{
	common.HexToAddress("0xaC97153e7ce86fB3e61681b969698AF7C22b4B12"),
	common.HexToAddress("0x084F933B6401a72291246B5B5eD46218a68773e6"),
	common.HexToAddress("0x8dD580271D823CBDC4a1C6153f69Dad594C521Fd"),
	common.HexToAddress("0x2D0Ed226891E256d94F1071E2F94FBcDC9060E14"),
}

/*
router : 0x5023882f4d1ec10544fcb2066abe9c1645e95aa0
factory: 0xC831A5cBfb4aC2Da5ed5B194385DFD9bF5bFcBa7

## tokens
0x21be370D5312f44cB42ce377BC9b8a0cEF1A4C83,WFTM

0x04068DA6C83AFCFA0e13ba15A6696662335D5B75,FTM官方部署的USDC
0x1B6382DBDEa11d97f24495C9A90b7c88469134a4,Axelar Wrapped USDC
0x2F733095B80A04b38b0D10cC884524a3d09b836a,Bridged USDC (USDC.e) wormhole
0x28a92dde19D9989F39A49905d7C9C2FAc7799bDf,Stargate bridge(layer0)
*/
var weiPerEther = new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
var usdcDecimalMul = new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil)
var pairs = map[common.Address]*Pair{
	// USDC/WFTM
	common.HexToAddress("0xaC97153e7ce86fB3e61681b969698AF7C22b4B12"): {
		addr:              common.HexToAddress("0xaC97153e7ce86fB3e61681b969698AF7C22b4B12"),
		token0Addr:        common.HexToAddress("0x04068DA6C83AFCFA0e13ba15A6696662335D5B75"),
		token1Addr:        common.HexToAddress("0x21be370D5312f44cB42ce377BC9b8a0cEF1A4C83"),
		decimalsMul0:      usdcDecimalMul,
		decimalsMul1:      weiPerEther,
		quoteIsStableCoin: false,
	},
	// axlUSDC/WFTM
	common.HexToAddress("0x084F933B6401a72291246B5B5eD46218a68773e6"): {
		addr:              common.HexToAddress("0x084F933B6401a72291246B5B5eD46218a68773e6"),
		token0Addr:        common.HexToAddress("0x1B6382DBDEa11d97f24495C9A90b7c88469134a4"),
		token1Addr:        common.HexToAddress("0x21be370D5312f44cB42ce377BC9b8a0cEF1A4C83"),
		decimalsMul0:      usdcDecimalMul,
		decimalsMul1:      weiPerEther,
		quoteIsStableCoin: false,
	},
	// WFTM/USDC(stargate)
	common.HexToAddress("0x8dD580271D823CBDC4a1C6153f69Dad594C521Fd"): {
		addr:              common.HexToAddress("0x8dD580271D823CBDC4a1C6153f69Dad594C521Fd"),
		token0Addr:        common.HexToAddress("0x21be370D5312f44cB42ce377BC9b8a0cEF1A4C83"),
		token1Addr:        common.HexToAddress("0x28a92dde19D9989F39A49905d7C9C2FAc7799bDf "),
		decimalsMul0:      weiPerEther,
		decimalsMul1:      usdcDecimalMul,
		quoteIsStableCoin: true,
	},
	// WFTM/USDC.e(wormhole)
	common.HexToAddress("0x2D0Ed226891E256d94F1071E2F94FBcDC9060E14"): {
		addr:              common.HexToAddress("0x2D0Ed226891E256d94F1071E2F94FBcDC9060E14"),
		token0Addr:        common.HexToAddress("0x21be370D5312f44cB42ce377BC9b8a0cEF1A4C83"),
		token1Addr:        common.HexToAddress("0x2F733095B80A04b38b0D10cC884524a3d09b836a"),
		decimalsMul0:      weiPerEther,
		decimalsMul1:      usdcDecimalMul,
		quoteIsStableCoin: true,
	},
}

func main() {
	log.SetFlags(log.Lmicroseconds | log.Lshortfile)

	// Initialize HTTP client
	client, err := rpc.Dial(nodeURL)
	if err != nil {
		log.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}

	pairAbi, err := abi.JSON(strings.NewReader(pairAbiStr))
	if err != nil {
		log.Fatalf("Failed to parse contract ABI: %v", err)
	}

	// Query initial reserves
	queryReserves(&pairAbi, client)

	// Initialize WebSocket client
	wsClient, err := rpc.Dial(wsNodeURL)
	if err != nil {
		log.Fatalf("Failed to connect to the Ethereum websocket client: %v", err)
	}
	return
	// Subscribe to Sync and Swap events
	subscribeEvents(pairAbi, wsClient, pairAddresses)
	panic("unreachable")
}

/*
## 单次 rpc 请求
curl -X POST https://rpcapi.fantom.network \
-H "Content-Type: application/json" \
-d '{
	"jsonrpc": "2.0",
	"method": "eth_call",
	"params": [
		{
			"to": "0xaC97153e7ce86fB3e61681b969698AF7C22b4B12",
			"data": "0x0902f1ac"
		},
		"latest"
	],
	"id": 1
}'
{"jsonrpc":"2.0","id":1,"result":"0x000000000000000000000000000000000000000000000000000000135d10239b00000000000000000000000000000000000000000000049e145dd82cd75b9d5500000000000000000000000000000000000000000000000000000000669e4462"}
## 批量 rpc 请求
```python
import requests
json = [
    {
        "jsonrpc": "2.0",
        "method": "eth_call",
        "params": [
            {"to": "0xaC97153e7ce86fB3e61681b969698AF7C22b4B12", "data": "0x0902f1ac"},
            "latest",
        ],
        "id": 1,
    },
    {
        "jsonrpc": "2.0",
        "method": "eth_call",
        "params": [
            {"to": "0x084F933B6401a72291246B5B5eD46218a68773e6", "data": "0x0902f1ac"},
            "latest",
        ],
        "id": 2,
    },
]
r=requests.post("https://rpcapi.fantom.network", json=json)
print(r.text)
```
*/
// 如何一次请求查询四个Pair的getReserve? 一种是直接rpc muticall,还有一种使用合约实现muticall
// rpc.Client 没有 ethclient.Client.CallContract
func queryReserves(pairAbi *abi.ABI, client *rpc.Client) {
	method, exists := pairAbi.Methods["getReserves"]
	if !exists {
		log.Fatal("pairAbi.Methods")
	}
	methodIdSignature := hexutil.Encode(hexutil.Bytes(method.ID))
	log.Println("method.Sig", method.Sig, "methodIdSignature", methodIdSignature, "method.ID")

	batch := make([]rpc.BatchElem, len(pairAddresses))
	// responses := make([]Reserves, len(pairAddresses))
	for i, addr := range pairAddresses {
		_ = addr
		batch[i] = rpc.BatchElem{
			Method: "eth_call",
			Args: []interface{}{
				/*ethereum.CallMsg{
					To: (*common.Address)(addr.Bytes()),
					Data:          method.ID,
				},*/
				map[string]string{
					"to": addr.Hex(),
					"data": methodIdSignature,
				},
				"latest",
			},
			// You are using []byte for the Result, but it’s often safer to use a hexutil.Bytes type or directly handle it as string to avoid encoding issues
			Result: new(hexutil.Bytes),
			// Result: &responses[i],
		}
	}
	err := client.BatchCall(batch)
	if err != nil {
		log.Fatalf("Batch call failed: %v", err)
	}
	// Process the results
	for i, elem := range batch {
		pairAddress := pairAddresses[i]
		// res := elem.Result.(*string)
		// log.Printf("%s\n", *res)
		if elem.Error != nil {
			log.Fatalf("Error fetching reserves for pair %s: %v", pairAddress, elem.Error, )
			continue
		}

		// Unpack the result
		reserveData := (*elem.Result.(*hexutil.Bytes))
		outputs, err := pairAbi.Unpack("getReserves", reserveData)
		if err != nil {
			log.Fatalf("Failed to unpack reserves data for pair %s: %v", pairAddress, err)
		}
		// reserve := elem.Result.(Reserves)

		// Extract reserves
		reserve0 := outputs[0].(*big.Int)
		reserve1 := outputs[1].(*big.Int)
		blockTimestampLast := outputs[2].(uint32)

		pair := pairs[pairAddress]
		pair.reserve = Reserves{
			Reserve0:           reserve0,
			Reserve1:           reserve1,
			BlockTimestampLast: blockTimestampLast,
		}
		amount0 := pair.amount0()
		amount1 := pair.amount1()
		amountF0 := pair.amountFloat0()
		amountF1 := pair.amountFloat1()
		price := pair.price()
		priceF := pair.priceFloat()
		log.Printf("pair %s rest init: price=%f amount0=%f amount1=%f priceF=%f amountF0=%f amountF1=%f\n", pairAddress.Hex(), price, amount0, amount1, priceF, amountF0, amountF1)
	}
}

/*
13:25:58.134841 main.go:105: topic Sync = 0x1c411e9a96e071241c2f21f7726b17ae89e3cab4c78be50e062b03a9fffbbad1
13:25:58.134973 main.go:106: topic Swap = 0xd78ad95fa46c994b6551d0da85fc275fe613ce37657fb8d5e3d130840159d822
*/
func subscribeEvents(contract abi.ABI, wsClient *rpc.Client, pairAddresses []common.Address) {
	ethClient := ethclient.NewClient(wsClient)

	log.Printf("topic %s = %s\n", syncEventStr, contract.Events[syncEventStr].ID)
	log.Printf("topic %s = %s\n", swapEventStr, contract.Events[swapEventStr].ID)
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

/*
2024/07/21 07:47:01 main.go:132: Received log: {Address:0x2D0Ed226891E256d94F1071E2F94FBcDC9060E14 Topics:[0x1c411e9a96e071241c2f21f7726b17ae89e3cab4c78be50e062b03a9fffbbad1] Data:[0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 2 14 59 251 135 147 62 3 17 47 218 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 1 44 34 148 73 28] BlockNumber:86354646 TxHash:0x534d7d16b35bf078fb681a54794ed51fafdb88993df76e9c93b9e1b242513540 TxIndex:1 BlockHash:0x0004801c00001dcfd0982594eccebf02fec83d1bd34a5a5f3326f9f7540e3983 Index:2 Removed:false}
2024/07/21 07:47:01 main.go:148: Updated reserves for 0x2D0Ed226891E256d94F1071E2F94FBcDC9060E14: {2485071252506902170513370 1289070332188}
2024/07/21 07:47:01 main.go:132: Received log: {Address:0x2D0Ed226891E256d94F1071E2F94FBcDC9060E14 Topics:[0xd78ad95fa46c994b6551d0da85fc275fe613ce37657fb8d5e3d130840159d822 0x0000000000000000000000005023882f4d1ec10544fcb2066abe9c1645e95aa0 0x0000000000000000000000002c846bcb8aa71a7f90cc5c7731c7a7716a51616e] Data:[0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 21 173 145 185 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 37 242 115 147 61 181 112 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0] BlockNumber:86354646 TxHash:0x534d7d16b35bf078fb681a54794ed51fafdb88993df76e9c93b9e1b242513540 TxIndex:1 BlockHash:0x0004801c00001dcfd0982594eccebf02fec83d1bd34a5a5f3326f9f7540e3983 Index:3 Removed:false}
*/
func handleLog(contract abi.ABI, vLog types.Log) {
	// Explicitly log the event type for debugging purposes
	log.Printf("Received log: %+v\n", vLog)
	switch vLog.Topics[0].Hex() {
	case contract.Events[syncEventStr].ID.Hex(): // EventSignature
		var syncEvent struct {
			Reserve0 *big.Int
			Reserve1 *big.Int
		}
		err := contract.UnpackIntoInterface(&syncEvent, syncEventStr, vLog.Data)
		if err != nil {
			log.Fatalf("Failed to unpack Sync event: %v", err)
		}
		pairAddress := vLog.Address
		pairs[pairAddress].reserve = Reserves{
			Reserve0: syncEvent.Reserve0,
			Reserve1: syncEvent.Reserve1,
		}
		log.Printf("Updated reserves for %s: %v\n", pairAddress.Hex(), pairs[pairAddress])
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
		currentReserves, exists := pairs[pairAddress]
		if !exists {
			log.Printf("No reserves found for address %s", pairAddress.Hex())
			return
		}
		_ = currentReserves

		// Update reserves based on swap event
		// updatedReserves := Reserves{
		// 	Reserve0: new(big.Int).Set(currentReserves.Reserve0),
		// 	Reserve1: new(big.Int).Set(currentReserves.Reserve1),
		// }

		// updatedReserves.Reserve0.Sub(updatedReserves.Reserve0, swapEvent.Amount0Out)
		// updatedReserves.Reserve0.Add(updatedReserves.Reserve0, swapEvent.Amount0In)

		// updatedReserves.Reserve1.Sub(updatedReserves.Reserve1, swapEvent.Amount1Out)
		// updatedReserves.Reserve1.Add(updatedReserves.Reserve1, swapEvent.Amount1In)

		// pairs[pairAddress].reserve = updatedReserves
		// log.Printf("Updated reserves for %s after swap: %v", pairAddress.Hex(), pairs[pairAddress])
	}
}
