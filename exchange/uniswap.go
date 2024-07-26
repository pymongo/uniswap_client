package exchange

import (
	"arbitrage/model"
	"context"
	"crypto/ecdsa"
	"log"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/tyler-smith/go-bip32"
	"github.com/tyler-smith/go-bip39"
)

const (
	rpcUrl  = "https://rpcapi.fantom.network"
	wsUrl   = "wss://wsapi.fantom.network/" // 不支持 ws 的 rpc 服务商就填空的 url
	chainId = 250                           // FTM
)

var (
	pairAddr = common.HexToAddress("0x084F933B6401a72291246B5B5eD46218a68773e6")
	usdcAddr = common.HexToAddress("0x1B6382DBDEa11d97f24495C9A90b7c88469134a4") // axlUsdc
	// wftmAddr = common.HexToAddress("0x21be370D5312f44cB42ce377BC9b8a0cEF1A4C83")
	weiPerEther    = new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	usdcDecimalMul = new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil)
)

type UniBroker struct {
	privKey *ecdsa.PrivateKey
	addr    common.Address
	bboCh   chan model.Bbo
	rest    *ethclient.Client
}

func NewUniBroker(key string, bboCh chan model.Bbo) UniBroker {
	var err error

	var privateKeyBytes []byte
	if key[:2] == "0x" || len(key) == 64 {
		// 不能拿 contains 空格判断是不是助记词，很可能私钥里面就有多个空格 byte
		privateKeyBytes, err = hexutil.Decode(key)
		if err != nil {
			log.Fatalln(key, err)
		}
	} else {
		privateKeyBytes = mnemonic2PrivateKey(key, 60)
	}

	privateKey, err := crypto.ToECDSA(privateKeyBytes)
	if err != nil {
		log.Fatalf("Failed to convert to ECDSA private key: %v", err)
	}
	// web3.Web3().eth.account.from_key('addr').address
	publicKey := privateKey.Public().(*ecdsa.PublicKey)
	address := crypto.PubkeyToAddress(*publicKey)

	rest, err := ethclient.Dial(rpcUrl)
	if err != nil {
		log.Fatalln(err)
	}

	params, err := BalanceOf.Inputs.Pack(address)
	if err != nil {
		log.Fatalln(err)
	}
	msg := ethereum.CallMsg{
		To: &usdcAddr,
		// abi.go return append(method.ID, arguments...), nil
		Data: append(BalanceOf.ID, params...),
	}
	resp, err := rest.CallContract(context.Background(), msg, nil)
	if err != nil {
		log.Fatalln(err)
	}
	values, err := BalanceOf.Outputs.UnpackValues(resp)
	if err != nil {
		log.Fatalln(err)
	}
	var usdcBitInt *big.Int
	err = BalanceOf.Outputs.Copy(&usdcBitInt, values)
	if err != nil {
		log.Fatalln(err)
	}
	usdcF64, _ := new(big.Float).SetInt(usdcBitInt).Float64()
	usdc := usdcF64 / 1e6
	ethWei, err := rest.BalanceAt(context.Background(), address, nil) // nil for the latests block
	if err != nil {
		log.Fatalln(err)
	}
	ethWeiF64, _ := new(big.Float).SetInt(ethWei).Float64()
	eth := ethWeiF64 / 1e18
	log.Println("eth = ", eth, "usdc = ", usdc)

	// blockId, err := rest.BlockNumber(context.Background())
	// if err != nil {
	// 	log.Fatalln(err)
	// }
	// block, err := rest.BlockByNumber(context.Background(), big.NewInt(int64(blockId)))
	// if err != nil {
	// 	log.Fatalln(err)
	// }
	// log.Printf("%#v", block)

	return UniBroker{
		privKey: privateKey,
		addr:    address,
		bboCh:   bboCh,
		rest:    rest,
	}
}

func (u *UniBroker) Mainloop() {
	if len(wsUrl) != 0 {
		go func() {
			log.Println("eth rpc wsUrl exist, subscribe Uniswap log/event")
			u.subscribeEvents(wsUrl)
		}()
	}
	for {
		u.queryReserves()
		time.Sleep(200 * time.Microsecond)
	}
}

var pairs = map[common.Address]*UniPair{
	pairAddr: {
		addr:                pairAddr,
		name:                "axlUSDC/WFTM",
		decimalsMul0:        usdcDecimalMul,
		decimalsMul1:        weiPerEther,
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

func mnemonic2PrivateKey(mnemonic string, slipp44CoinType uint32) []byte {
	seed := bip39.NewSeed(mnemonic, "")
	masterKey, err := bip32.NewMasterKey(seed)
	if err != nil {
		log.Fatalln(err)
	}
	purpose, err := masterKey.NewChildKey(bip32.FirstHardenedChild + 44)
	if err != nil {
		log.Fatalf("Failed to derive purpose key: %v", err)
	}
	coinType, err := purpose.NewChildKey(bip32.FirstHardenedChild + slipp44CoinType)
	if err != nil {
		log.Fatalf("Failed to derive coin type key: %v", err)
	}
	account, err := coinType.NewChildKey(bip32.FirstHardenedChild + 0)
	if err != nil {
		log.Fatalf("Failed to derive account key: %v", err)
	}
	change, err := account.NewChildKey(0)
	if err != nil {
		log.Fatalf("Failed to derive change key: %v", err)
	}

	addressIndex, err := change.NewChildKey(0)
	if err != nil {
		log.Fatalf("Failed to derive address index key: %v", err)
	}
	return addressIndex.Key
}

type UniPair struct {
	addr common.Address
	name string // 只是用于日志打印
	// token0Addr   common.Address
	// token1Addr   common.Address
	reserve0            *big.Int
	reserve1            *big.Int
	decimalsMul0        *big.Int // e.g. 1e18
	decimalsMul1        *big.Int
	priceIsQuoteDivBase bool
}

func (pair *UniPair) amount0() float64 {
	reserve := new(big.Int).Set(pair.reserve0)
	reserve.Div(reserve, pair.decimalsMul0)
	amount := new(big.Float).SetInt(reserve)
	float, _ := amount.Float64()
	return float
}
func (pair *UniPair) amount1() float64 {
	reserve := new(big.Int).Set(pair.reserve1)
	reserve.Div(reserve, pair.decimalsMul1)
	amount := new(big.Float).SetInt(reserve)
	float, _ := amount.Float64()
	return float
}
func (pair *UniPair) price() float64 {
	amount0 := pair.amount0()
	amount1 := pair.amount1()

	if pair.priceIsQuoteDivBase {
		return amount1 / amount0
	} else {
		return amount0 / amount1
	}
}
func (pair *UniPair) bbo() model.Bbo {
	price := pair.price()
	return model.Bbo{
		Ask:    price,
		Bid:    price,
		TimeMs: time.Now().UnixMilli(),
	}
}

func (u *UniBroker) queryReserves() {
	methodIdSignature := hexutil.Encode(hexutil.Bytes(GetReserves.ID))
	batch := make([]rpc.BatchElem, len(pairs))
	i := 0
	for addr := range pairs {
		_ = addr
		batch[i] = rpc.BatchElem{
			Method: "eth_call",
			Args: []interface{}{
				map[string]string{
					"to":   addr.Hex(),
					"data": methodIdSignature,
				},
				"latest",
			},
			Result: new(hexutil.Bytes),
		}
		i++
	}
	err := u.rest.Client().BatchCall(batch)
	if err != nil {
		log.Fatalf("Batch call failed: %v", err)
	}
	for i, elem := range batch {
		pairAddress := pairAddresses[i]
		if elem.Error != nil {
			log.Fatalf("Error fetching reserves for pair %s: %v", pairAddress, elem.Error)
			continue
		}
		reserveData := (*elem.Result.(*hexutil.Bytes))
		values, err := GetReserves.Outputs.UnpackValues(reserveData)
		if err != nil {
			log.Fatalln(err)
		}
		var reserve GetReservesOutput
		err = GetReserves.Outputs.Copy(&reserve, values)
		if err != nil {
			log.Fatalln(err)
		}
		pair := pairs[pairAddress]
		pair.reserve0 = reserve.Reserve0
		pair.reserve1 = reserve.Reserve1
		// price := pair.price()
		u.bboCh <- pair.bbo()
	}
}

func (u *UniBroker) subscribeEvents(wsUrl string) {
	c, err := rpc.DialWebsocket(context.Background(), wsUrl, "")
	if err != nil {
		log.Fatalln(err)
	}
	client := ethclient.NewClient(c)
	eventsAbi := PairEventsAbi{
		Swap:     NewEventAbi(&PairAbi, "Swap"),
		Sync:     NewEventAbi(&PairAbi, "Sync"),
		Burn:     NewEventAbi(&PairAbi, "Burn"),
		Mint:     NewEventAbi(&PairAbi, "Mint"),
		Transfer: NewEventAbi(&PairAbi, "Transfer"),
	}
	query := ethereum.FilterQuery{
		Addresses: pairAddresses,
		// Topic就是EventSignature的意思用于标识事件的唯一标识符。每个事件都有一个固定的签名
		Topics: [][]common.Hash{{
			eventsAbi.Swap.Id,
			eventsAbi.Sync.Id,
			eventsAbi.Burn.Id,
			eventsAbi.Mint.Id,
			eventsAbi.Transfer.Id,
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
			u.handleLog(&eventsAbi, vLog)
		}
	}
}
func (u *UniBroker) handleLog(eventsAbi *PairEventsAbi, logEvt types.Log) {
	pairAddress := logEvt.Address
	pair := pairs[pairAddress]
	switch logEvt.Topics[0] {
	case eventsAbi.Sync.Id: // EventSignature
		values, err := eventsAbi.Sync.Arg.UnpackValues(logEvt.Data)
		if err != nil {
			log.Fatalf("Failed to unpack Sync event: %v", err)
		}
		var reserve SyncEvent
		err = eventsAbi.Sync.Arg.Copy(&reserve, values)
		if err != nil {
			log.Fatalln(err)
		}
		pair.reserve0 = reserve.Reserve0
		pair.reserve1 = reserve.Reserve1
		// log.Printf("ws_event Sync %s price %f\n", pair.name, pair.price())
		u.bboCh <- pair.bbo()
	case eventsAbi.Swap.Id:
		values, err := eventsAbi.Swap.Arg.UnpackValues(logEvt.Data)
		if err != nil {
			log.Fatalf("Failed to unpack Swap event: %v", err)
		}
		var swap SwapEvent
		err = eventsAbi.Swap.Arg.Copy(&swap, values)
		if err != nil {
			log.Fatalln(err)
		}
		pair.reserve0.Sub(pair.reserve0, swap.Amount0Out)
		pair.reserve0.Add(pair.reserve0, swap.Amount0In)
		pair.reserve1.Sub(pair.reserve1, swap.Amount1Out)
		pair.reserve1.Add(pair.reserve1, swap.Amount1In)
		// log.Printf("ws_event Swap %s price %f\n", pair.name, pair.price())
		u.bboCh <- pair.bbo()
	case eventsAbi.Burn.Id:
		values, err := eventsAbi.Burn.Arg.UnpackValues(logEvt.Data)
		if err != nil {
			log.Fatalf("Failed to unpack Burn event: %v", err)
		}
		var data BurnEvent
		err = eventsAbi.Burn.Arg.Copy(&data, values)
		if err != nil {
			log.Fatalln(err)
		}
		log.Printf("ws_event Burn %s Topics %v, data %#v price %f\n", pair.name, logEvt.Topics, data, pair.price())
	case eventsAbi.Mint.Id:
		values, err := eventsAbi.Mint.Arg.UnpackValues(logEvt.Data)
		if err != nil {
			log.Fatalf("Failed to unpack Mint event: %v", err)
		}
		var data MintEvent
		err = eventsAbi.Mint.Arg.Copy(&data, values)
		if err != nil {
			// 14:56:18.233005 main.go:497: abi: field value can't be found in the given value
			log.Fatalln(err, logEvt.Data)
		}
		log.Printf("ws_event Mint %s Topics %v, data %#v price %f\n", pair.name, logEvt.Topics, data, pair.price())
	case eventsAbi.Transfer.Id:
		values, err := eventsAbi.Transfer.Arg.UnpackValues(logEvt.Data)
		if err != nil {
			log.Fatalf("Failed to unpack Transfer event: %v", err)
		}
		var data TransferEvent
		err = eventsAbi.Transfer.Arg.Copy(&data, values)
		if err != nil {
			log.Println(err)
		}
		log.Printf("ws_event Transfer %s Topics %v, data %#v price %f\n", pair.name, logEvt.Topics, data, pair.price())
	}
}
