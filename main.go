package main

import "fmt"
import "github.com/globbie/gnode/pkg/knd-bridge"

func main() {
	shard, _ := bridge.ShardNew()

	fmt.Println(shard)
}
