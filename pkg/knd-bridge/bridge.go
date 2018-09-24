package bridge

// #cgo CFLAGS: -I${SRCDIR}/knowdy/core/include
// #cgo CFLAGS: -I${SRCDIR}/knowdy/libs/gsl-parser/include
// #cgo LDFLAGS: ${SRCDIR}/knowdy/build/lib/libcore_static.a
// #cgo LDFLAGS: ${SRCDIR}/knowdy/build/lib/libglb-lib_static.a
// #include <knd_shard.h>
import "C"
import "unsafe"

type Shard struct {
	shard *C.struct_kndShard
}

func ShardNew() (*Shard, error) {
	var shard *C.struct_kndShard = nil

	C.kndShard_new(unsafe.Pointer(&shard), unsafe.Pointer(nil))
	ret := &Shard{
		shard: shard,
	}
	return ret, nil
}
