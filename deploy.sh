#!/bin/bash
set -exu
bin=latency
target=""
r=${r:-lb1}

go build binance/main.go
rsync -avz --progress main $r:main
ssh $r /root/main
