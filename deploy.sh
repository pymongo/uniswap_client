#!/bin/bash
set -exu
bin=latency
target=""
r=${r:-lb1}

# go build cmd/arbitrage/main.go
go build cmd/uniswap/main.go
rsync -avz --progress main $r:main
ssh $r /root/main
