package knowdy

import "testing"

const shard_cfg = `
{schema knd
      {agent 007}
      {path .}
      {user User}
      {schemas ../../config/knowdy-schemas/basic}

      {sid AUTH_SERVER_SID}
      {-- address localhost:10001 --}

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
	shard, err := New(shard_cfg)
	if err != nil {
		t.Error(err)
	}
	defer shard.Del()
}
