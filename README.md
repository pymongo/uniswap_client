```
go mod init uniswap
go get github.com/ethereum/go-ethereum
go get github.com/ethereum/go-ethereum/ethclient
#go get github.com/ethereum/go-ethereum/rpc
```

> go install github.com/ethereum/go-ethereum/cmd/abigen@latest

https://etherscan.io/address/0xB4e16d0168e52d35CaCD2c6185b44281Ec28C9Dc#code

https://github.com/mantlenetworkio/mantle/blob/main/mt-batcher/Makefile

op 源码 v1.7 之前 可以看到有个 bindings 相关库

```
op-bindings/bindgen/utils.go
168:    cmd := exec.Command("abigen", "--abi", abiFilePath, "--bin", bytecodeFilePath, "--pkg", goPackageName, "--type", contractName, "--out", outFilePath)
```

> abigen --abi exchange/bindings/uniswapv2_pair.abi --out exchange/bindings/uniswapv2_pair.go --pkg bindings

> abigen --abi exchange/bindings/erc20.abi --out exchange/bindings/erc20.go --pkg bindings

不加 --type 参数的话，所有结构体命名都是 包名 也就是 Bindings 这样会冲突

```
abigen --abi exchange/bindings/uniswapv2_router.abi --out exchange/bindings/uniswapv2_router.go --type UniswapV2Router --pkg bindings
abigen --abi exchange/bindings/uniswapv2_factory.abi --out exchange/bindings/uniswapv2_factory.go --type UniswapV2Factory --pkg bindings
abigen --abi exchange/bindings/uniswapv2_pair.abi --out exchange/bindings/uniswapv2_pair.go --type UniswapV2Pair --pkg bindings

# 不能用 ETH USDC 合约的 abi 里面用的是 Read as Proxy
abigen --abi exchange/bindings/erc20.abi --out exchange/bindings/erc20.go --type Erc20 --pkg bindings
```

> solc --bin --abi --optimize -o exchange/bindings/ exchange/bindings/SwapHelper.sol

> forge init --no-commit

## 编译运行

> go build cmd/uniswap/main.go && ./main temp/config.ftm.toml

## 为什么没用 go.work

go.work 是用来管理 项目内有多个 go.mod 的子module的，也就是 monorepo

```
go install golang.org/x/lint/golint
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```
