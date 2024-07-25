package exchange

import (
	"arbitrage/model"
	"context"
	"crypto/ecdsa"
	"log"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/tyler-smith/go-bip32"
	"github.com/tyler-smith/go-bip39"
)

const (
	//lint:ignore U1000 ignore
	rpcUrl  = "https://rpcapi.fantom.network"
	wsUrl   = "wss://wsapi.fantom.network/"
	chainId = 250 // FTM
)

var (
	pairAddr = common.HexToAddress("0x084F933B6401a72291246B5B5eD46218a68773e6")
	usdcAddr = common.HexToAddress("0x1B6382DBDEa11d97f24495C9A90b7c88469134a4") // axlUsdc
	// wftmAddr = common.HexToAddress("0x21be370D5312f44cB42ce377BC9b8a0cEF1A4C83")
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

	// log.Println(hexutil.Encode(privateKeyBytes))
	privateKey, err := crypto.ToECDSA(privateKeyBytes)
	if err != nil {
		log.Fatalf("Failed to convert to ECDSA private key: %v", err)
	}
	// web3.Web3().eth.account.from_key('addr').address
	publicKey := privateKey.Public().(*ecdsa.PublicKey)
	address := crypto.PubkeyToAddress(*publicKey)
	// log.Println("addr", address, "key", key, "Encode(privateKeyBytes)", hexutil.Encode(privateKeyBytes), "len", len(privateKeyBytes))

	rest, err := ethclient.Dial(rpcUrl)
	if err != nil {
		log.Fatalln(err)
	}

	// in function calls, 20 bytes addr must padding left to 32 bytes
	params, err := Erc20BalanceOf.Inputs.Pack(address)
	if err != nil {
		log.Fatalln(err)
	}
	msg := ethereum.CallMsg{
		To:   &usdcAddr,
		// abi.go return append(method.ID, arguments...), nil
		Data: append(Erc20BalanceOf.ID, params...),
	}
	resp, err := rest.CallContract(context.Background(), msg, nil)
	if err != nil {
		log.Fatalln(err)
	}
	values, err := Erc20BalanceOf.Outputs.UnpackValues(resp)
	if err != nil {
		log.Fatalln(err)
	}
	var usdcBitInt *big.Int
	err = Erc20BalanceOf.Outputs.Copy(&usdcBitInt, values)
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

	return UniBroker{
		privKey: privateKey,
		addr:    address,
		bboCh:   bboCh,
		rest:    rest,
	}
}

func (bn *UniBroker) Mainloop(symbols []string) {

}

func mnemonic2PrivateKey(mnemonic string, slipp44CoinType uint32) []byte {
	seed := bip39.NewSeed(mnemonic, "")
	masterKey, err := bip32.NewMasterKey(seed)
	if err != nil {
		log.Fatalln(err)
	}
	// Derive the first account (index 0)
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
