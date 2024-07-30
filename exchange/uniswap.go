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
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
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

const (
	gasLimit = uint64(21000) // Gas limit for standard ETH transfer
)
// var (
// 	weiPerEther    = new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
// 	usdcDecimalMul = new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil)
// )

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

	var spaceCount = 0
	for _,char := range []byte(conf.PrivateKey) {
		if char == ' ' {
			spaceCount += 1
		}
	}
	key := conf.PrivateKey
	var privateKeyBytes []byte
	if spaceCount == 12 || spaceCount == 24 {
		privateKeyBytes = mnemonic2PrivateKey(key, 60)
	} else {
		// 不能拿 contains 空格判断是不是助记词，很可能私钥里面就有多个空格 byte
		privateKeyBytes, err = hexutil.Decode(key)
		if err != nil {
			log.Fatalln(key, err)
		}
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
	for _,pair := range u.conf.Pairs {
		// log.Printf("%#v", pair)
		log.Printf("Pair %s price=%f %s amount=%f,%f", pair.Addr, pair.Price(), pair.Addr, pair.Amount0(), pair.Amount1())
	}
	log.Printf("%s wallet %fUSDC %fETH", u.addr, u.Usdc, u.Eth)
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

func (u *UniBroker) queryReserves() error {
	methodIdSignature := hexutil.Encode(hexutil.Bytes(GetReserves.ID))
	batch := make([]rpc.BatchElem, len(u.conf.Pairs))
	for i, pair := range u.conf.Pairs {
		batch[i] = rpc.BatchElem{
			Method: "eth_call",
			Args: []interface{}{
				map[string]string{
					"to":   pair.Addr.Hex(),
					"data": methodIdSignature,
				},
				"latest",
			},
			Result: new(hexutil.Bytes),
		}
	}
	err := u.client.Client().BatchCall(batch)
	if err != nil {
		// log.Fatalf("Batch call failed: %v", err)
		return err
	}
	for i, elem := range batch {
		pair := &u.conf.Pairs[i]
		pairAddress := pair.Addr
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
		if reserve.Reserve0 == nil {
			log.Fatalln("reserve.Reserve0 == nil")
		}
		pair.Reserve0 = reserve.Reserve0
		pair.Reserve1 = reserve.Reserve1
		// log.Printf("%#v", pair)
		u.bboCh <- pair.Bbo()
	}
	// log.Printf("%#v",  u.conf.Pairs[0])
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
	pairAddresses := make([]common.Address, len(u.conf.Pairs))
	for i := range u.conf.Pairs {
		pairAddresses[i] = u.conf.Pairs[i].Addr
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
	var pair config.UniPair
	for _, p := range u.conf.Pairs {
		if p.Addr == pairAddress {
			pair = p
			break;
		}
	}
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
		pair.Reserve0 = reserve.Reserve0
		pair.Reserve1 = reserve.Reserve1
		// log.Printf("ws_event Sync %s price %f\n", pair.name, pair.price())
		u.bboCh <- pair.Bbo()
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
		pair.Reserve0.Sub(pair.Reserve0, swap.Amount0Out)
		pair.Reserve0.Add(pair.Reserve0, swap.Amount0In)
		pair.Reserve1.Sub(pair.Reserve1, swap.Amount1Out)
		pair.Reserve1.Add(pair.Reserve1, swap.Amount1In)
		// log.Printf("ws_event Swap %s price %f\n", pair.name, pair.price())
		u.bboCh <- pair.Bbo()
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
		log.Printf("ws_event Burn %s Topics %v, data %#v price %f\n", pair.Name, logEvt.Topics, data, pair.Price())
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
		log.Printf("ws_event Mint %s Topics %v, data %#v price %f\n", pair.Name, logEvt.Topics, data, pair.Price())
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
		log.Printf("ws_event Transfer %s Topics %v, data %#v price %f\n", pair.Name, logEvt.Topics, data, pair.Price())
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
	u.nonce += 1
	receipt, err := bind.WaitMined(context.Background(), u.client, signedTx)
	if err != nil {
		return err
	}
	log.Printf("transfer %f to %s receipt %#v", amountEther, to, receipt)
	if receipt.Status == types.ReceiptStatusFailed {
		return errors.New("TransferEth tx fail")
	}
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
func (u *UniBroker) Swap(pair *config.UniPair, side model.Side, amount float64) error {
	amount0Out := big.NewInt(0)
	amount1Out := big.NewInt(0)
	var baseCcyMul *big.Float
	if pair.QuoteIsToken1 {
		baseCcyMul = big.NewFloat(pair.DecimalsMul0)
	} else {
		baseCcyMul = big.NewFloat(pair.DecimalsMul1)
	}
	baseCcyAmount, _ := new(big.Float).Mul(big.NewFloat(amount), baseCcyMul).Int(nil)
	log.Println("amount", amount, "baseCcyMul", baseCcyMul, "baseCcyAmount", baseCcyAmount)
	if baseCcyAmount == big.NewInt(0) {
		log.Fatalln("amount is zero")
	}
	if side == model.SideBuy {
		if pair.QuoteIsToken1 {
			amount0Out = baseCcyAmount
		} else {
			amount1Out = baseCcyAmount
		}
	} else {
		// 卖出 ETH 获得 USDC 的情况复杂点, 要用 x*y=k 公式算出可获得多少 USDC
		k := new(big.Int).Mul(pair.Reserve0, pair.Reserve1)
		if pair.QuoteIsToken1 {
			newBaseReserve := new(big.Int).Add(pair.Reserve0, baseCcyAmount)
			newQuoteReserve := new(big.Int).Div(k, newBaseReserve)
			log.Println(pair.Reserve0, "*", pair.Reserve1, "->", newBaseReserve, "*", newQuoteReserve)
			SellQuoteAmount := new(big.Int).Sub(newQuoteReserve, pair.Reserve1)
			amount1Out = SellQuoteAmount
		} else {
			newBaseReserve := new(big.Int).Add(pair.Reserve1, baseCcyAmount)
			newQuoteReserve := new(big.Int).Div(k, newBaseReserve)
			log.Println(pair.Reserve0, "*", pair.Reserve1, "->", newQuoteReserve, "*", newBaseReserve)
			SellQuoteAmount := new(big.Int).Sub(newQuoteReserve, pair.Reserve0)
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
	tx := types.NewTransaction(u.nonce, pair.Addr, big.NewInt(0), 8*gasLimit, u.gasPrice, data)
    signedTx, err := types.SignTx(tx, types.NewEIP155Signer(u.chainId), u.privKey)
	if err != nil {
		log.Fatalln(err)
	}
	// txhash := signedTx.Hash().Hex()
	err = u.client.SendTransaction(context.Background(), signedTx)
	if err != nil {
		return err
	}
	u.nonce += 1
	receipt, err := bind.WaitMined(context.Background(), u.client, signedTx)
	if err != nil {
		return err
	}
	log.Printf("swap %s side=%d amount=%f amount0Out=%d amount0Out=%d %#v", pair.Name, side, amount, amount0Out, amount1Out, receipt)
	if receipt.Status == types.ReceiptStatusFailed {
		return errors.New("swap tx fail")
	}
	return err
}

func (u *UniBroker) BuyEth(pairIdx int, amount float64) error {
	pair := &u.conf.Pairs[0]
	var baseCcyMul *big.Float
	if pair.QuoteIsToken1 {
		baseCcyMul = big.NewFloat(pair.DecimalsMul0)
	} else {
		baseCcyMul = big.NewFloat(pair.DecimalsMul1)
	}
	baseCcyAmount, _ := new(big.Float).Mul(big.NewFloat(amount), baseCcyMul).Int(nil)	
	k := new(big.Int).Mul(pair.Reserve0, pair.Reserve1)
	// 计算最大滑点相关 必须设置滑点保护否则怕被MEV夹
	var amountInMax *big.Int
	if pair.QuoteIsToken1 {
		newBaseReserve := new(big.Int).Sub(pair.Reserve0, baseCcyAmount)
		newQuoteReserve := new(big.Int).Div(k, newBaseReserve)
		//lint:ignore SA4006 not_used
		amountInMax = new(big.Int).Sub(newQuoteReserve, pair.Reserve1)
	} else {
		newBaseReserve := new(big.Int).Sub(pair.Reserve1, baseCcyAmount)
		newQuoteReserve := new(big.Int).Div(k, newBaseReserve)
		//lint:ignore SA4006 not_used
		amountInMax = new(big.Int).Sub(newQuoteReserve, pair.Reserve0)
	}
	// 手续费 0.2% 滑点 0.1%
	fee := 0.0019
	sliapge := 0.01
	amountInMax = big.NewInt((int64)(pair.Price() * amount * 1e6 * (1+fee+sliapge)))
	// amountInMax = big.NewInt(520000)

	routerClient, err := bindings.NewUniswapV2RouterTransactor(u.conf.RouterAddr, u.client)
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
		GasLimit:  gasLimit * 10,
	}
	path := []common.Address {
		pair.Token0Addr,
		pair.Token1Addr,
	}
	deadline := big.NewInt(time.Now().Add(30 * time.Hour).Unix())
	tx, err := routerClient.SwapExactTokensForTokens(&opts, baseCcyAmount, amountInMax, path, u.addr, deadline)
	if err != nil {
		log.Println(err)
		return err
	}
	u.nonce += 1 // 不管成功失败只要给过 Gas 费 nonce 就自增
	receipt, err := bind.WaitMined(context.Background(), u.client, tx)
	if err != nil {
		log.Println(err)
		return err
	}
	log.Println(baseCcyAmount, amountInMax, path, u.addr, deadline, receipt.TxHash, receipt.Logs)
	if receipt.Status == types.ReceiptStatusFailed {
		return errors.New("swap tx fail")
	}
	return nil
}

func (u *UniBroker) RouterSellEth() {
	// if side == model.SideBuy {
	// 	if pair.QuoteIsToken1 {
	// 		newBaseReserve := new(big.Int).Sub(pair.Reserve0, baseCcyAmount)
	// 		newQuoteReserve := new(big.Int).Div(k, newBaseReserve)
	// 		amountInMax = new(big.Int).Sub(pair.Reserve1, newQuoteReserve)
	// 	} else {
	// 		newBaseReserve := new(big.Int).Sub(pair.Reserve1, baseCcyAmount)
	// 		newQuoteReserve := new(big.Int).Div(k, newBaseReserve)
	// 		amountInMax = new(big.Int).Sub(pair.Reserve0, newQuoteReserve)
	// 	}
	// } else {
	// 	if pair.QuoteIsToken1 {
	// 		newBaseReserve := new(big.Int).Add(pair.Reserve0, baseCcyAmount)
	// 		newQuoteReserve := new(big.Int).Div(k, newBaseReserve)
	// 		amountInMax = new(big.Int).Sub(newQuoteReserve, pair.Reserve1)
	// 	} else {
	// 		newBaseReserve := new(big.Int).Add(pair.Reserve1, baseCcyAmount)
	// 		newQuoteReserve := new(big.Int).Div(k, newBaseReserve)
	// 		amountInMax = new(big.Int).Sub(newQuoteReserve, pair.Reserve0)
	// 	}	
	// }	
	panic("TODO swapExactETHForTokens")
}

func (u *UniBroker) DeployContract() {
	// 加载合约的ABI
	abiData, err := os.ReadFile("exchange/bindings/SwapHelper.abi")
	if err != nil {
		log.Fatalf("Failed to read ABI file: %v", err)
	}
	if len(abiData) > 0 {
		panic("DeployContract would deploy fail please do not use")
	}
	parsedABI, err := abi.JSON(strings.NewReader(string(abiData)))
	if err != nil {
		log.Fatalf("Failed to parse contract ABI: %v", err)
	}
	// 获取合约部署的字节码
	bytecode, err := os.ReadFile("exchange/bindings/SwapHelper.bin")
	if err != nil {
		log.Fatalf("Failed to read bytecode file: %v", err)
	}
	// 创建交易
	opts, err := bind.NewKeyedTransactorWithChainID(u.privKey, u.chainId)
	if err != nil {
		log.Fatalln(err)
	}
	opts.Nonce = big.NewInt((int64)(u.nonce))
	opts.GasLimit = uint64(11000000)
	opts.GasPrice = u.gasPrice
	// 部署合约
	address, tx, _, err := bind.DeployContract(opts, parsedABI, bytecode, u.client)
	if err != nil {
		log.Fatalf("Failed to deploy contract: %v", err)
	}
	u.nonce += 1 // 不管成功失败只要给过 Gas 费 nonce 就自增
	addr, err := bind.WaitDeployed(context.Background(), u.client, tx)
	if err != nil {
		log.Fatalln(err)
	}
	log.Println(address, addr)
}
