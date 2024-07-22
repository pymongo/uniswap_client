```
go mod init uniswap
go get github.com/ethereum/go-ethereum
go get github.com/ethereum/go-ethereum/ethclient
#go get github.com/ethereum/go-ethereum/rpc
```

```
go install golang.org/x/lint/golint
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

```
can you help me write write a golang code uniswapV2 ws subscribe pair Sync/Swap event to watch reserve change?
I want to subscribe these four contract 0xaC97153e7ce86fB3e61681b969698AF7C22b4B12,0x084F933B6401a72291246B5B5eD46218a68773e6,0x8dD580271D823CBDC4a1C6153f69Dad594C521Fd,0x2D0Ed226891E256d94F1071E2F94FBcDC9060E14 both are UniswapV2Pair contract.
first http rpc query four contract current reserve to init value(you can use global var to store address to reserve data),
then when ws subscribe Sync/Swap, on Sync message replace reserve value in memory and Swap event increment update reserve value in memory
```
