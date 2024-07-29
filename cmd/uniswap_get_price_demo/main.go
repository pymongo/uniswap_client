package main

import (
	"arbitrage/exchange"
	"arbitrage/exchange/bindings"
	"context"
	_ "embed"
	"log"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

// go:embed IUniswapV2Pair.abi.json
var pairAbiStr string

const (
	nodeURL   = "https://rpcapi.fantom.network"
	wsNodeURL = "wss://wsapi.fantom.network/"
)

type Pair struct {
	addr         common.Address
	name         string // 只是用于日志打印
	token0Addr   common.Address
	token1Addr   common.Address
	decimalsMul0 *big.Int // e.g. 1e18
	decimalsMul1 *big.Int
	reserve      exchange.GetReservesOutput
	// e.g. quote_coin/token1 is USDC so price is reserve0/reserve1, Vice versa
	priceIsQuoteDivBase bool
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

	if pair.priceIsQuoteDivBase {
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

	if pair.priceIsQuoteDivBase {
		return amount1 / amount0
	} else {
		return amount0 / amount1
	}
}


// https://ethereum.org/en/developers/docs/data-structures-and-encoding/rlp/ unmarshal
// func (b *Reserves) DecodeRLP(s *rlp.Stream) error {
// 	panic("haha")
// }

/*
router : 0x5023882f4d1ec10544fcb2066abe9c1645e95aa0
factory: 0xC831A5cBfb4aC2Da5ed5B194385DFD9bF5bFcBa7

factory getPair 查询不到会返回 0x0000000000000000000000000000000000000000

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
	// common.HexToAddress("0xaC97153e7ce86fB3e61681b969698AF7C22b4B12"): {
	// 	addr:              common.HexToAddress("0xaC97153e7ce86fB3e61681b969698AF7C22b4B12"),
	// 	name:              "USDC/WFTM",
	// 	token0Addr:        common.HexToAddress("0x04068DA6C83AFCFA0e13ba15A6696662335D5B75"),
	// 	token1Addr:        common.HexToAddress("0x21be370D5312f44cB42ce377BC9b8a0cEF1A4C83"),
	// 	decimalsMul0:      usdcDecimalMul,
	// 	decimalsMul1:      weiPerEther,
	// 	quoteIsStableCoin: false,
	// },
	common.HexToAddress("0x084F933B6401a72291246B5B5eD46218a68773e6"): {
		addr:              common.HexToAddress("0x084F933B6401a72291246B5B5eD46218a68773e6"),
		name:              "axlUSDC/WFTM",
		token0Addr:        common.HexToAddress("0x1B6382DBDEa11d97f24495C9A90b7c88469134a4"),
		token1Addr:        common.HexToAddress("0x21be370D5312f44cB42ce377BC9b8a0cEF1A4C83"),
		decimalsMul0:      usdcDecimalMul,
		decimalsMul1:      weiPerEther,
		priceIsQuoteDivBase: false,
	},
	common.HexToAddress("0x8dD580271D823CBDC4a1C6153f69Dad594C521Fd"): {
		addr:              common.HexToAddress("0x8dD580271D823CBDC4a1C6153f69Dad594C521Fd"),
		name:              "WFTM/lzUSDC", // stargate
		token0Addr:        common.HexToAddress("0x21be370D5312f44cB42ce377BC9b8a0cEF1A4C83"),
		token1Addr:        common.HexToAddress("0x28a92dde19D9989F39A49905d7C9C2FAc7799bDf "),
		decimalsMul0:      weiPerEther,
		decimalsMul1:      usdcDecimalMul,
		priceIsQuoteDivBase: true,
	},
	common.HexToAddress("0x2D0Ed226891E256d94F1071E2F94FBcDC9060E14"): {
		addr:              common.HexToAddress("0x2D0Ed226891E256d94F1071E2F94FBcDC9060E14"),
		name:              "WFTM/USDC.e", // wormhole
		token0Addr:        common.HexToAddress("0x21be370D5312f44cB42ce377BC9b8a0cEF1A4C83"),
		token1Addr:        common.HexToAddress("0x2F733095B80A04b38b0D10cC884524a3d09b836a"),
		decimalsMul0:      weiPerEther,
		decimalsMul1:      usdcDecimalMul,
		priceIsQuoteDivBase: true,
	},
	common.HexToAddress("0xCE102955A36f148e034C6Fc8Aac0a2ea86f0B281"): {
		addr:              common.HexToAddress("0xCE102955A36f148e034C6Fc8Aac0a2ea86f0B281"),
		name:              "axlUSDC/WIGO", // rug, priceF=0.021381 amountF0=0.064873 amountF1=3.034192
		token0Addr:        common.HexToAddress("0x1B6382DBDEa11d97f24495C9A90b7c88469134a4"),
		token1Addr:        common.HexToAddress("0xE992bEAb6659BFF447893641A378FbbF031C5bD6"),
		decimalsMul0:      usdcDecimalMul,
		decimalsMul1:      weiPerEther,
		priceIsQuoteDivBase: false,
	},
	common.HexToAddress("0x96bDF4d9fb8dB9FcD1E0CA146faBD891f2F1A96d"): {
		addr:              common.HexToAddress("0x96bDF4d9fb8dB9FcD1E0CA146faBD891f2F1A96d"),
		name:              "USDC/WIGO",
		token0Addr:        common.HexToAddress("0x04068DA6C83AFCFA0e13ba15A6696662335D5B75"),
		token1Addr:        common.HexToAddress("0xE992bEAb6659BFF447893641A378FbbF031C5bD6"),
		decimalsMul0:      usdcDecimalMul,
		decimalsMul1:      weiPerEther,
		priceIsQuoteDivBase: false,
	},
	common.HexToAddress("0xB66E5c89EbA830B31B3dDcc468dD50b3256737c5"): {
		addr:              common.HexToAddress("0xB66E5c89EbA830B31B3dDcc468dD50b3256737c5"),
		name:              "USDC.e/WIGO",
		decimalsMul0:      usdcDecimalMul,
		decimalsMul1:      weiPerEther,
		priceIsQuoteDivBase: false,
	},
	common.HexToAddress("0xAA606265Df9d29687876B500c18d5DDf1a66a91E"): {
		addr:              common.HexToAddress("0xAA606265Df9d29687876B500c18d5DDf1a66a91E"),
		name:              "lzUSDC/WIGO",
		decimalsMul0:      usdcDecimalMul,
		decimalsMul1:      weiPerEther,
		priceIsQuoteDivBase: false,
	},
	common.HexToAddress("0xB66E5c89EbA830B31B3dDcc468dD50b3256737c5"): {
		addr:              common.HexToAddress("0xB66E5c89EbA830B31B3dDcc468dD50b3256737c5"),
		name:              "WFTM/WIGO",
		decimalsMul0:      usdcDecimalMul,
		decimalsMul1:      weiPerEther,
		priceIsQuoteDivBase: false,
	},	
}
func getPairAddr() []common.Address {
	p := make([]common.Address, len(pairs))
	i := 0
	for key := range pairs {
		p[i] = key
		i += 1
	}
	return p
}
var pairAddresses = getPairAddr()

func main() {
	log.SetFlags(log.Lmicroseconds | log.Lshortfile)

	// Initialize HTTP client
	client, err := rpc.Dial(nodeURL)
	if err != nil {
		log.Fatalf("Failed to connect to the Ethereum client: %v", err)
	}

	var pairAbi = exchange.PairAbi

	// Query initial reserves
	queryReserves(client)

	// Initialize WebSocket client
	wsClient, err := rpc.Dial(wsNodeURL)
	if err != nil {
		log.Fatalf("Failed to connect to the Ethereum websocket client: %v", err)
	}
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
// 如何一次请求查询四个Pair的getReserve? 一种是直接rpc muticall,还有一种使用合约实现multicall
// rpc.Client 没有 ethclient.Client.CallContract
// multicall 合约示例 https://ftmscan.com/address/0xb828c456600857abd4ed6c32facc607bd0464f4f#code
func queryReserves(client *rpc.Client) {
	method := exchange.GetReserves
	// method.ID = crypto.Keccak256([]byte(method.Signature))[:4]
	methodIdSignature := hexutil.Encode(hexutil.Bytes(method.ID))
	// log.Println("method.Sig", method.Sig, "methodIdSignature", methodIdSignature, "method.ID")

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
					"to":   addr.Hex(),
					"data": methodIdSignature,
				},
				"latest",
			},
			// You are using []byte for the Result, but it’s often safer to use a hexutil.Bytes type or directly handle it as string to avoid encoding issues
			Result: new(hexutil.Bytes),
			// Result: &Reserves{},
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
			log.Fatalf("Error fetching reserves for pair %s: %v", pairAddress, elem.Error)
			continue
		}

		// Unpack the result
		// reserveData := (*elem.Result.(*Reserves))

		reserveData := (*elem.Result.(*hexutil.Bytes))

		values, err := method.Outputs.UnpackValues(reserveData)
		if err != nil {
			log.Fatalln(err)
		}
		var reserve exchange.GetReservesOutput
		err = method.Outputs.Copy(&reserve, values)
		if err != nil {
			log.Fatalln(err)
		}

		// reserve0 := values[0].(*big.Int)
		// reserve1 := values[1].(*big.Int)
		// blockTimestampLast := values[2].(uint32)
		pair := pairs[pairAddress]
		pair.reserve = reserve
		amount0 := pair.amount0()
		amount1 := pair.amount1()
		amountF0 := pair.amountFloat0()
		amountF1 := pair.amountFloat1()
		price := pair.price()
		priceF := pair.priceFloat()
		log.Printf("pair %s rest init: price=%f amount0=%f amount1=%f priceF=%f amountF0=%f amountF1=%f\n", pair.name, price, amount0, amount1, priceF, amountF0, amountF1)
	}
}

/*
13:25:58.134841 main.go:105: topic Sync = 0x1c411e9a96e071241c2f21f7726b17ae89e3cab4c78be50e062b03a9fffbbad1
13:25:58.134973 main.go:106: topic Swap = 0xd78ad95fa46c994b6551d0da85fc275fe613ce37657fb8d5e3d130840159d822
*/
func subscribeEvents(contract abi.ABI, wsClient *rpc.Client, pairAddresses []common.Address) {
	client := ethclient.NewClient(wsClient)
	abiCtx := exchange.PairEventsAbi {
		Swap: exchange.NewEventAbi(&contract, "Swap"),
		Sync: exchange.NewEventAbi(&contract, "Sync"),
		Burn: exchange.NewEventAbi(&contract, "Burn"),
		Mint: exchange.NewEventAbi(&contract, "Mint"),
		Transfer: exchange.NewEventAbi(&contract, "Transfer"),
	}
	query := ethereum.FilterQuery{
		Addresses: pairAddresses,
		// Topic就是EventSignature的意思用于标识事件的唯一标识符。每个事件都有一个固定的签名
		Topics: [][]common.Hash{{
			abiCtx.Swap.Id,
			abiCtx.Sync.Id,
			abiCtx.Burn.Id,
			abiCtx.Mint.Id,
			abiCtx.Transfer.Id,
			// Approval 不会发生 token 数量变化
		}},
	}
	logs := make(chan types.Log)
	sub, err := client.SubscribeFilterLogs(context.Background(), query, logs)
	if err != nil {
		log.Fatalf("Failed to subscribe to logs: %v", err)
	}
	// defer sub.Unsubscribe()
	for {
		select {
		case err := <-sub.Err():
			log.Fatalf("Subscription error: %v", err)
		case vLog := <-logs:
			handleLog(&abiCtx, vLog)
		}
	}
}

/*
2024/07/21 07:47:01 main.go:132: Received log: {Address:0x2D0Ed226891E256d94F1071E2F94FBcDC9060E14 Topics:[0x1c411e9a96e071241c2f21f7726b17ae89e3cab4c78be50e062b03a9fffbbad1] Data:[0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 2 14 59 251 135 147 62 3 17 47 218 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 1 44 34 148 73 28] BlockNumber:86354646 TxHash:0x534d7d16b35bf078fb681a54794ed51fafdb88993df76e9c93b9e1b242513540 TxIndex:1 BlockHash:0x0004801c00001dcfd0982594eccebf02fec83d1bd34a5a5f3326f9f7540e3983 Index:2 Removed:false}
2024/07/21 07:47:01 main.go:148: Updated reserves for 0x2D0Ed226891E256d94F1071E2F94FBcDC9060E14: {2485071252506902170513370 1289070332188}
2024/07/21 07:47:01 main.go:132: Received log: {Address:0x2D0Ed226891E256d94F1071E2F94FBcDC9060E14 Topics:[0xd78ad95fa46c994b6551d0da85fc275fe613ce37657fb8d5e3d130840159d822 0x0000000000000000000000005023882f4d1ec10544fcb2066abe9c1645e95aa0 0x0000000000000000000000002c846bcb8aa71a7f90cc5c7731c7a7716a51616e] Data:[0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 21 173 145 185 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 37 242 115 147 61 181 112 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0] BlockNumber:86354646 TxHash:0x534d7d16b35bf078fb681a54794ed51fafdb88993df76e9c93b9e1b242513540 TxIndex:1 BlockHash:0x0004801c00001dcfd0982594eccebf02fec83d1bd34a5a5f3326f9f7540e3983 Index:3 Removed:false}
后面的两个 topic 通常是涉及到的代币地址或与事件相关的其他索引参数。例如，在 Uniswap 中，第二个 topic 可能是流动性提供者的地址，第三个 topic 可能是其它参与者或合约的地址
*/
func handleLog(abiCtx *exchange.PairEventsAbi, logEvt types.Log) {
	pairAddress := logEvt.Address
	pair := pairs[pairAddress]
	switch logEvt.Topics[0] {
	case abiCtx.Sync.Id: // EventSignature
		values, err := abiCtx.Sync.Arg.UnpackValues(logEvt.Data)
		if err != nil {
			log.Fatalf("Failed to unpack Sync event: %v", err)
		}
		var reserve bindings.UniswapV2PairSync
		err = abiCtx.Sync.Arg.Copy(&reserve, values)
		if err != nil {
			log.Fatalln(err)
		}
		pair.reserve.Reserve0 = reserve.Reserve0
		pair.reserve.Reserve1 = reserve.Reserve1
		log.Printf("ws_event Sync %s price %f\n", pair.name, pair.price())
	case abiCtx.Swap.Id:
		values, err := abiCtx.Swap.Arg.UnpackValues(logEvt.Data)
		if err != nil {
			log.Fatalf("Failed to unpack Swap event: %v", err)
		}
		var swap bindings.UniswapV2PairSwap
		err = abiCtx.Swap.Arg.Copy(&swap, values)
		if err != nil {
			log.Fatalln(err)
		}
		reserve := pair.reserve
		reserve.Reserve0.Sub(reserve.Reserve0, swap.Amount0Out)
		reserve.Reserve0.Add(reserve.Reserve0, swap.Amount0In)
		reserve.Reserve1.Sub(reserve.Reserve1, swap.Amount1Out)
		reserve.Reserve1.Add(reserve.Reserve1, swap.Amount1In)
		log.Printf("ws_event Swap %s price %f\n", pair.name, pair.price())
	case abiCtx.Burn.Id:
		values, err := abiCtx.Burn.Arg.UnpackValues(logEvt.Data)
		if err != nil {
			log.Fatalf("Failed to unpack Burn event: %v", err)
		}
		var data bindings.UniswapV2PairBurn
		err = abiCtx.Burn.Arg.Copy(&data, values)
		if err != nil {
			log.Fatalln(err)
		}		
		log.Printf("ws_event Burn %s Topics %v, data %#v price %f\n", pair.name, logEvt.Topics, data, pair.price())
	case abiCtx.Mint.Id:
		values, err := abiCtx.Mint.Arg.UnpackValues(logEvt.Data)
		if err != nil {
			log.Fatalf("Failed to unpack Mint event: %v", err)
		}
		var data bindings.UniswapV2PairMint
		err = abiCtx.Mint.Arg.Copy(&data, values)
		if err != nil {
			// 14:56:18.233005 main.go:497: abi: field value can't be found in the given value
			log.Fatalln(err, logEvt.Data)
		}		
		log.Printf("ws_event Mint %s Topics %v, data %#v price %f\n", pair.name, logEvt.Topics, data, pair.price())
	case abiCtx.Transfer.Id:
		values, err := abiCtx.Transfer.Arg.UnpackValues(logEvt.Data)
		if err != nil {
			log.Fatalf("Failed to unpack Transfer event: %v", err)
		}
		var data bindings.UniswapV2PairTransfer
		err = abiCtx.Transfer.Arg.Copy(&data, values)
		if err != nil {
			log.Println(err)
		}		
		log.Printf("ws_event Transfer %s Topics %v, data %#v price %f\n", pair.name, logEvt.Topics, data, pair.price())
	}
}
