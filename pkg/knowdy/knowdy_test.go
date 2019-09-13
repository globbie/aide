package knowdy

import (
	"log"
	"testing"
)

const shardCfg = `
{schema knd
	{db-path .}
	{schema-path testdata/system-schemas 
		{user User
			{base-repo shared-repo
				{schema-path testdata/shared-schemas}
			}
		}
	}
	{memory
		{-- TODO: check limits 0 1? --}
		{max_base_pages        20000}
		{max_small_x4_pages    4500}
		{max_small_x2_pages    150000}
		{max_small_pages       23000}
		{max_tiny_pages        200000}
	}
}
`

func TestShard(t *testing.T) {
	shard, err := New(shardCfg, 1)
	if err != nil {
		t.Error(err)
	}
	result, _, err := shard.RunTask("{task {tid 123}}")
	if err != nil {
		t.Error(err)
	}
	log.Println("empty task result:", result)
	defer shard.Del()
}
