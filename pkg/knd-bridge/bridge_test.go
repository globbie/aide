package bridge

import "testing"

func TestShard(t *testing.T) {
	shard, err := ShardNew("knowdy/etc/knowdy/learner.gsl")
	if err != nil {
		t.Error(err)
	}
	defer shard.Close()
}
