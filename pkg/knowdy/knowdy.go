package knowdy 

// #cgo CFLAGS: -I${SRCDIR}/knowdy/core/include
// #cgo CFLAGS: -I${SRCDIR}/knowdy/libs/gsl-parser/include
// #cgo LDFLAGS: ${SRCDIR}/knowdy/build/lib/libcore_static.a
// #cgo LDFLAGS: ${SRCDIR}/knowdy/build/lib/libglb-lib_static.a
// #cgo LDFLAGS: ${SRCDIR}/knowdy/build/lib/libgsl-parser_static.a
// #include <knd_shard.h>
// static void kndShard_del__(struct kndShard *shard)
// {
//     if (shard) {
//         shard->del(shard);
//     }
// }
import "C"

//import "unsafe"
import (
	"errors"
)

type Shard struct {
	shard *C.struct_kndShard
}

func New(conf string) (*Shard, error) {
	var shard *C.struct_kndShard = nil
	errCode := C.kndShard_new(&shard, C.CString(conf), C.size_t(len(conf)))
	if errCode != C.int(0) {
		return nil, errors.New("could not create shard struct")
	}
	ret := &Shard{
		shard: shard,
	}
	return ret, nil
}

func (s *Shard) Del() error {
	C.kndShard_del__(nil)
	return nil
}

func (s *Shard) RunTask(task string) (string, error) {
	a := "asdfasdf"
	_ = len(a)
	return "", nil
}

