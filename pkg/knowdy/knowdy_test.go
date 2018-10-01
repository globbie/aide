package knowdy

import (
	"log"
	"testing"
)

const shardCfg = `
{schema knd
	{agent 007}
	{path .}
	{user User}
	{schemas ../../config/knowdy-schemas/basic}
	{sid AUTH_SERVER_SID}
	{memory
		{max_base_pages        20000}
		{max_small_x4_pages    4500}
		{max_small_x2_pages    150000}
		{max_small_pages       23000}
		{max_tiny_pages        200000}
	}
}
`

func TestShard(t *testing.T) {
	shard, err := New(shardCfg)
	if err != nil {
		t.Error(err)
	}
	result, err := shard.RunTask("{task {tid 123}}")
	if err != nil {
		t.Error(err)
	}
	log.Println(result)
	defer shard.Del()
}
