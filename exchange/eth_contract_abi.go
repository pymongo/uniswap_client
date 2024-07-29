package exchange

import (
	"arbitrage/exchange/bindings"
	"log"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

// go 文件名命名规范都是 snake_case 例如 go 源码的 cgo_disabled.go

// go:embed bindings/uniswapv2_pair.abi
// var IUniswapV2PairAbiStr string
var PairAbi, err1 = abi.JSON(strings.NewReader(bindings.UniswapV2PairMetaData.ABI))
var Erc20Abi, err2 = abi.JSON(strings.NewReader(bindings.Erc20MetaData.ABI))
var BalanceOf = Erc20Abi.Methods["balanceOf"]
var GetReserves = PairAbi.Methods["getReserves"]
var Swap = PairAbi.Methods["swap"]

// json/eth_rlp decode/Unmarshal 都是通过运行时反射匹配字段，必须要大写才能找到字段 abi package uses reflection to match the ABI event parameters with struct fields by name.
type GetReservesOutput struct {
	Reserve0 *big.Int
	Reserve1 *big.Int
	// 这个字段虽然不用，但也必须定义，否则会报错 abi: field _blockTimestampLast can't be found in the given value
	BlockTimestampLast uint32
}
type EventAbi struct {
	Arg abi.Arguments
	Id  common.Hash
}
type PairEventsAbi struct {
	Swap     EventAbi
	Sync     EventAbi
	Burn     EventAbi
	Mint     EventAbi
	Transfer EventAbi
}

func NewEventAbi(pairAbi *abi.ABI, event string) EventAbi {
	evt := pairAbi.Events[event]
	if evt.Name == "" {
		panic(event)
	}
	// log.Println("newEvtCtx", event, evt.ID.Hex())
	return EventAbi{
		Arg: evt.Inputs,
		Id:  evt.ID,
	}
}

func CheckAbiMethods() {
	if err1 != nil {
		log.Fatalln(err1)
	}
	if err2 != nil {
		log.Fatalln(err2)
	}
	if BalanceOf.Name == "" {
		log.Fatalf("BalanceOf %#v", Erc20Abi.Methods)
	}
	if GetReserves.Name == "" {
		log.Fatalln("GetReserves")
	}
	if Swap.Name == "" {
		log.Fatalln("Swap")
	}
}
