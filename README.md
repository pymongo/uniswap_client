```
go mod init uniswap
go get github.com/ethereum/go-ethereum
go get github.com/ethereum/go-ethereum/ethclient
#go get github.com/ethereum/go-ethereum/rpc
```

## 为什么没用 go.work

go.work 是用来管理 项目内有多个 go.mod 的子module的，也就是 monorepo

```
go install golang.org/x/lint/golint
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```
