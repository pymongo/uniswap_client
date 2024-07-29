package exchange

import (
	"arbitrage/config"
	"arbitrage/exchange/bindings"
	"arbitrage/model"
	"context"
	"crypto/ecdsa"
	"errors"
	"log"
	"math"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/tyler-smith/go-bip32"
	"github.com/tyler-smith/go-bip39"
)

// const (
// 	wsUrl  = "wss://wsapi.fantom.network/" // 不支持 ws 的 rpc 服务商就填空的 url
// )

const (
	gasLimit = uint64(21000) // Gas limit for standard ETH transfer
)
var (
	PairAddr = common.HexToAddress("0x084F933B6401a72291246B5B5eD46218a68773e6")
	// wftmAddr = common.HexToAddress("0x21be370D5312f44cB42ce377BC9b8a0cEF1A4C83")
	weiPerEther    = new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	usdcDecimalMul = new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil)
)

type UniBroker struct {
	privKey *ecdsa.PrivateKey
	addr    common.Address
	nonce   uint64
	Eth     float64 // gas coin
	Usdc    float64 // USDC or USDT
	chainId *big.Int
	client    *ethclient.Client
	gasPrice *big.Int
	conf *config.Config
	bboCh   chan model.Bbo
}

func NewUniBroker(conf *config.Config, bboCh chan model.Bbo) UniBroker {
	CheckAbiMethods()
	var err error

	key := conf.PrivateKey
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
	// privateKey = crypto.ToECDSAUnsafe(privateKeyBytes)
	if err != nil {
		log.Fatalf("Failed to convert to ECDSA private key: %v", err)
	}
	// web3.Web3().eth.account.from_key('addr').address
	publicKey := privateKey.Public().(*ecdsa.PublicKey)
	address := crypto.PubkeyToAddress(*publicKey)

	rest, err := ethclient.Dial(conf.RpcUrl)
	if err != nil {
		log.Fatalln(err)
	}

	nonce, err := rest.NonceAt(context.Background(), address, nil)
	if err != nil {
		log.Fatalln(err)
	}
	// rest.NetworkID()
	chainId, err := rest.ChainID(context.Background())
	if err != nil {
		log.Fatalln(err)
	}
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
		client:    rest,
		nonce:   nonce,
		chainId: chainId,
		conf: conf,
	}
}

func (u *UniBroker) Mainloop() {
	err := u.queryReserves()
	if err != nil {
		log.Fatalln(err)
	}

	err = u.queryBalanceGasPrice()
	if err != nil {
		log.Fatalln(err)
	}
	if len(u.conf.WsUrl) != 0 {
		go func() {
			log.Println("eth rpc wsUrl exist, subscribe Uniswap log/event")
			for {
				err = u.subscribeEvents(u.conf.WsUrl)
				if err != nil {
					log.Println("err", err)
				}
				time.Sleep(1 * time.Second)
			}
		}()
	}
	log.Printf("链上资产 %fUSDC %fETH", u.Usdc, u.Eth)
	go func() {
		for {
			time.Sleep(250 * time.Microsecond)
			err = u.queryReserves()
			if err != nil {
				log.Println("err", err)
			}
		}
	}()
	go func() {
		for {
			time.Sleep(1200 * time.Microsecond)
			err = u.queryBalanceGasPrice()
			if err != nil {
				log.Println("err", err)
			}
		}
	}()
}

var Pairs = map[common.Address]*UniPair{
	PairAddr: {
		addr:                PairAddr,
		name:                "axlUSDC/WFTM",
		token0Addr:          common.HexToAddress("0x1B6382DBDEa11d97f24495C9A90b7c88469134a4"),
		token1Addr:          common.HexToAddress("0x21be370D5312f44cB42ce377BC9b8a0cEF1A4C83"),		
		decimalsMul0:        usdcDecimalMul,
		decimalsMul1:        weiPerEther,
		quoteIsToken1: false,
	},
}

func getPairAddr() []common.Address {
	p := make([]common.Address, len(Pairs))
	i := 0
	for key := range Pairs {
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
	token0Addr   common.Address
	token1Addr   common.Address
	reserve0            *big.Int
	reserve1            *big.Int
	decimalsMul0        *big.Int // e.g. 1e18
	decimalsMul1        *big.Int
	quoteIsToken1 bool // e.g. USDC/ETH is false
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

	if pair.quoteIsToken1 {
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

func (u *UniBroker) queryReserves() error {
	methodIdSignature := hexutil.Encode(hexutil.Bytes(GetReserves.ID))
	batch := make([]rpc.BatchElem, len(Pairs))
	i := 0
	for addr := range Pairs {
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
	err := u.client.Client().BatchCall(batch)
	if err != nil {
		// log.Fatalf("Batch call failed: %v", err)
		return err
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
		pair := Pairs[pairAddress]
		pair.reserve0 = reserve.Reserve0
		pair.reserve1 = reserve.Reserve1
		// price := pair.price()
		u.bboCh <- pair.bbo()
	}
	return nil
}

func (u *UniBroker) subscribeEvents(wsUrl string) error {
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
			// log.Fatalf("Subscription error: %v", err)
			return err
		case vLog := <-logs:
			u.handleLog(&eventsAbi, vLog)
		}
	}
}
func (u *UniBroker) handleLog(eventsAbi *PairEventsAbi, logEvt types.Log) {
	pairAddress := logEvt.Address
	pair := Pairs[pairAddress]
	switch logEvt.Topics[0] {
	case eventsAbi.Sync.Id: // EventSignature
		values, err := eventsAbi.Sync.Arg.UnpackValues(logEvt.Data)
		if err != nil {
			log.Fatalf("Failed to unpack Sync event: %v", err)
		}
		var reserve bindings.UniswapV2PairSync
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
		var swap bindings.UniswapV2PairSwap
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
		var data bindings.UniswapV2PairBurn
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
		var data bindings.UniswapV2PairMint
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
		var data bindings.UniswapV2PairTransfer
		err = eventsAbi.Transfer.Arg.Copy(&data, values)
		if err != nil {
			log.Println(err)
		}
		log.Printf("ws_event Transfer %s Topics %v, data %#v price %f\n", pair.name, logEvt.Topics, data, pair.price())
	}
}

func (u *UniBroker) queryBalanceGasPrice() error {
	batch := make([]rpc.BatchElem, 3)

	var ethBalance hexutil.Big
	batch[0] = rpc.BatchElem {
		Method: "eth_getBalance", // BalanceAt
		Args: []interface{} {
			u.addr,
			"latest",
		},
		Result: &ethBalance,
	}

	// hex number with leading zero digits
	// hexutil.Big 要求返回值没有 padding left 的零，eth 标准接口 gasPrice 这些都没有填充0
	var usdcBalance hexutil.Bytes
	params, err := BalanceOf.Inputs.Pack(u.addr) // 32byte addr with 0 padding left
	// log.Println(len(params), len(u.addr))
	if err != nil {
		log.Fatalln(err)
	}
	batch[1] = rpc.BatchElem {
		Method: "eth_call",
		// https://github.com/ethereum/go-ethereum/pull/15640/files
		// 用 data 或者 input 都行 data 是后面 rename 成 input 的
		Args: []interface{} {
			map[string]hexutil.Bytes {
				"to":   u.conf.UsdcAddr.Bytes(),
				"input": hexutil.Bytes(append(BalanceOf.ID, params...)),
			},
			"latest",
		},
		Result: &usdcBalance,
	}

	var gasPrice hexutil.Big
	batch[2] = rpc.BatchElem {
		Method: "eth_gasPrice",
		Result: &gasPrice,
	}
	
	err = u.client.Client().BatchCall(batch)
	if err != nil {
		return err
	}
	for i, elem := range batch {
		if elem.Error != nil {
			log.Printf("rpc BatchCall i=%d err=%#v", i, elem.Error)
			return elem.Error
		}
	}

	ethF64, _ := ethBalance.ToInt().Float64()
	u.Eth = ethF64 / 1e18
	usdcF64, _ :=  new(big.Int).SetBytes(usdcBalance).Float64()
	// gasPriceGwei, _ := gasPrice.ToInt().Float64()
	// log.Printf("eth %f, usdc %f, gasPrice %f gWei\n", ethF64, usdcF64, gasPriceGwei / 1e9)
	u.Usdc = usdcF64 / 1e6
	u.gasPrice = gasPrice.ToInt()
	return nil
}

func (u *UniBroker) TransferEth(amountEther float64) error {
	to := u.conf.DepositAddr
	amountWei := new(big.Int).SetInt64(int64(math.Floor(amountEther * 1e18)))
	tx := types.NewTransaction(u.nonce, to, amountWei, gasLimit, u.gasPrice, nil)
    signedTx, err := types.SignTx(tx, types.NewEIP155Signer(u.chainId), u.privKey)
	if err != nil {
		log.Fatalln(err)
	}
	// txhash := signedTx.Hash().Hex()
	err = u.client.SendTransaction(context.Background(), signedTx)
	if err != nil {
		return err
	}
	receipt, err := bind.WaitMined(context.Background(), u.client, signedTx)
	if err != nil {
		return err
	}
	log.Printf("transfer %f to %s receipt %#v", amountEther, to, receipt)
	if receipt.Status == types.ReceiptStatusFailed {
		return errors.New("TransferEth tx fail")
	}
	u.nonce += 1
	return err
}

// can you give a example for USDC/ETH UniswapV2Pair contract eth client to sell 0.001 ETH using golang interact with IUniswapV2Pair smart contract with swap abi?
/*
## IUniswapV2Pair swap Inputs 假设交易对是 ETH/USDC
- amount0Out 100 买入100 wei ETH
- amount1Out 0
- to 接收 ETH 的地址，一般填自己地址
- 任意的附加数据（calldata）在某些情况下用于 flash swaps。如果包含数据，则可能会触发更多复杂的操作，比如在执行完 swap 后立即调用接收者的合约以处理闪电贷逻辑
*/
// side 买卖操作 针对的是 base currency
func (u *UniBroker) Swap(pair *UniPair, side model.Side, amount float64) error {
	amount0Out := big.NewInt(0)
	amount1Out := big.NewInt(0)
	var baseCcyMul *big.Float
	if pair.quoteIsToken1 {
		baseCcyMul = new(big.Float).SetInt(pair.decimalsMul0)
	} else {
		baseCcyMul = new(big.Float).SetInt(pair.decimalsMul1)
	}
	baseCcyAmount, _ := new(big.Float).Mul(big.NewFloat(amount), baseCcyMul).Int(nil)
	log.Println("amount", amount, "baseCcyMul", baseCcyMul, "baseCcyAmount", baseCcyAmount)
	if baseCcyAmount == big.NewInt(0) {
		log.Fatalln("amount is zero")
	}
	if side == model.SideBuy {
		if pair.quoteIsToken1 {
			amount0Out = baseCcyAmount
		} else {
			amount1Out = baseCcyAmount
		}
	} else {
		// https://ftmscan.com/tx/0xd429b9a74d35f6d8a1c3dcfc455843afaf675c31454d5b01d47a3e2e11b1e7d3
		// 卖出 ETH 获得 USDC 的情况复杂点, 要用 x*y=k 公式算出可获得多少 USDC
		k := new(big.Int).Mul(pair.reserve0, pair.reserve1)
		if pair.quoteIsToken1 {
			newBaseReserve := new(big.Int).Add(pair.reserve0, baseCcyAmount)
			newQuoteReserve := new(big.Int).Div(k, newBaseReserve)
			log.Println(pair.reserve0, "*", pair.reserve1, "->", newBaseReserve, "*", newQuoteReserve)
			SellQuoteAmount := new(big.Int).Sub(newQuoteReserve, pair.reserve1)
			amount1Out = SellQuoteAmount
		} else {
			newBaseReserve := new(big.Int).Add(pair.reserve1, baseCcyAmount)
			newQuoteReserve := new(big.Int).Div(k, newBaseReserve)
			log.Println(pair.reserve0, "*", pair.reserve1, "->", newQuoteReserve, "*", newBaseReserve)
			SellQuoteAmount := new(big.Int).Sub(newQuoteReserve, pair.reserve0)
			amount0Out = SellQuoteAmount
		}
	}
	args, err := Swap.Inputs.Pack(amount0Out, amount1Out, u.addr, []byte{})
	if err != nil {
		log.Fatalln(err)	
	}
	data := make([]byte, 4 + len(args))
	copy(data, Swap.ID)
	copy(data[4:], args)
	tx := types.NewTransaction(u.nonce, pair.addr, big.NewInt(0), 8*gasLimit, u.gasPrice, data)
    signedTx, err := types.SignTx(tx, types.NewEIP155Signer(u.chainId), u.privKey)
	if err != nil {
		log.Fatalln(err)
	}
	// txhash := signedTx.Hash().Hex()
	err = u.client.SendTransaction(context.Background(), signedTx)
	if err != nil {
		return err
	}
	receipt, err := bind.WaitMined(context.Background(), u.client, signedTx)
	if err != nil {
		return err
	}
	log.Printf("swap %s side=%d amount=%f amount0Out=%d amount0Out=%d %#v", pair.name, side, amount, amount0Out, amount1Out, receipt)
	if receipt.Status == types.ReceiptStatusFailed {
		return errors.New("swap tx fail")
	}
	u.nonce += 1
	return err
}

func (u *UniBroker) BuyEth(pair *UniPair, amount float64) error {
	var baseCcyMul *big.Float
	if pair.quoteIsToken1 {
		baseCcyMul = new(big.Float).SetInt(pair.decimalsMul0)
	} else {
		baseCcyMul = new(big.Float).SetInt(pair.decimalsMul1)
	}
	baseCcyAmount, _ := new(big.Float).Mul(big.NewFloat(amount), baseCcyMul).Int(nil)	
	k := new(big.Int).Mul(pair.reserve0, pair.reserve1)
	// 计算最大滑点相关 必须设置滑点保护否则怕被MEV夹
	var amountInMax *big.Int
	if pair.quoteIsToken1 {
		newBaseReserve := new(big.Int).Sub(pair.reserve0, baseCcyAmount)
		newQuoteReserve := new(big.Int).Div(k, newBaseReserve)
		amountInMax = new(big.Int).Sub(newQuoteReserve, pair.reserve1)
	} else {
		newBaseReserve := new(big.Int).Sub(pair.reserve1, baseCcyAmount)
		newQuoteReserve := new(big.Int).Div(k, newBaseReserve)
		amountInMax = new(big.Int).Sub(newQuoteReserve, pair.reserve0)
	}

	routerClient, err := bindings.NewUniswapV2RouterTransactor(pair.addr, u.client)
	if err != nil {
		log.Fatalln(err)
	}
	opts := bind.TransactOpts{
		From:  u.addr,
		Nonce: big.NewInt((int64)(u.nonce)),
		Signer: func(a common.Address, tx *types.Transaction) (*types.Transaction, error) {
			signedTx, err := types.SignTx(tx, types.NewEIP155Signer(u.chainId), u.privKey)
			if err != nil {
				log.Fatalln(err)
			}
			return signedTx, nil
		},
		GasPrice:  u.gasPrice,
		GasLimit:  gasLimit * 8,
	}
	path := []common.Address {
		pair.token0Addr,
		pair.token1Addr,
	}
	deadline := big.NewInt(time.Now().Add(30 * time.Second).Unix())
	tx, err := routerClient.SwapTokensForExactETH(&opts, baseCcyAmount, amountInMax, path, u.addr, deadline)
	if err != nil {
		log.Println(err)
		return err
	}
	receipt, err := bind.WaitMined(context.Background(), u.client, tx)
	if err != nil {
		log.Println(err)
		return err
	}
	log.Println(baseCcyAmount, amountInMax, path, u.addr, deadline, receipt.TxHash)
	if receipt.Status == types.ReceiptStatusFailed {
		return errors.New("swap tx fail")
	}
	u.nonce += 1
	return nil
}

func (u *UniBroker) RouterSellEth() {
	// if side == model.SideBuy {
	// 	if pair.quoteIsToken1 {
	// 		newBaseReserve := new(big.Int).Sub(pair.reserve0, baseCcyAmount)
	// 		newQuoteReserve := new(big.Int).Div(k, newBaseReserve)
	// 		amountInMax = new(big.Int).Sub(pair.reserve1, newQuoteReserve)
	// 	} else {
	// 		newBaseReserve := new(big.Int).Sub(pair.reserve1, baseCcyAmount)
	// 		newQuoteReserve := new(big.Int).Div(k, newBaseReserve)
	// 		amountInMax = new(big.Int).Sub(pair.reserve0, newQuoteReserve)
	// 	}
	// } else {
	// 	if pair.quoteIsToken1 {
	// 		newBaseReserve := new(big.Int).Add(pair.reserve0, baseCcyAmount)
	// 		newQuoteReserve := new(big.Int).Div(k, newBaseReserve)
	// 		amountInMax = new(big.Int).Sub(newQuoteReserve, pair.reserve1)
	// 	} else {
	// 		newBaseReserve := new(big.Int).Add(pair.reserve1, baseCcyAmount)
	// 		newQuoteReserve := new(big.Int).Div(k, newBaseReserve)
	// 		amountInMax = new(big.Int).Sub(newQuoteReserve, pair.reserve0)
	// 	}	
	// }	
	panic("TODO swapExactETHForTokens")
}
