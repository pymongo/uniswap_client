package exchange

import (
	_ "embed"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

// go 文件名命名规范都是 snake_case 例如 go 源码的 cgo_disabled.go

//go:embed bindings/uniswapv2_pair.abi
var IUniswapV2PairAbiStr string
//go:embed bindings/erc20.abi
var erc20AbiStr string

var PairAbi, _ = abi.JSON(strings.NewReader(IUniswapV2PairAbiStr))
var Erc20Abi, _ = abi.JSON(strings.NewReader(erc20AbiStr))
var BalanceOf = Erc20Abi.Methods["balanceOf"]
var GetReserves = PairAbi.Methods["getReserves"]

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
