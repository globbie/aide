package knowdy

// #cgo CFLAGS: -I${SRCDIR}/knowdy/include
// #cgo CFLAGS: -I${SRCDIR}/knowdy/libs/glb-lib/include
// #cgo CFLAGS: -I${SRCDIR}/knowdy/libs/gsl-parser/include
// #cgo LDFLAGS: ${SRCDIR}/knowdy/build/lib/libknowdy_static.a
// #cgo LDFLAGS: ${SRCDIR}/knowdy/build/libs/glb-lib/lib/libglb-lib_static.a
// #cgo LDFLAGS: ${SRCDIR}/knowdy/build/libs/gsl-parser/lib/libgsl-parser_static.a
// #include <knd_shard.h>
// #include <knd_task.h>
// static void kndShard_del__(struct kndShard *shard)
// {
//     if (shard) {
//         knd_shard_del(shard);
//     }
// }
import "C"
import (
	"errors"
	"log"
	"unsafe"
)

type Shard struct {
	shard *C.struct_kndShard
}

func New(conf string) (*Shard, error) {
	var shard *C.struct_kndShard = nil

	log.Println(conf)

	errCode := C.knd_shard_new((**C.struct_kndShard)(&shard), C.CString(conf), C.size_t(len(conf)))
	if errCode != C.int(0) {
		return nil, errors.New("could not create shard struct")
	}
	errCode = C.knd_shard_serve((*C.struct_kndShard)(shard))
	if errCode != C.int(0) {
		return nil, errors.New("could not start Knowdy shard's service")
	}
        ret := &Shard{
		shard: shard,
	}
	return ret, nil
}

func (s *Shard) Del() error {
	C.kndShard_del__(s.shard)
	return nil
}

func taskTypeToStr(v C.int) string {
	switch v {
	case C.KND_GET_STATE:    return "get"
	case C.KND_SELECT_STATE: return "select"
	case C.KND_UPDATE_STATE: return "update"
	default:                 return "unknown"
	}
}

func (s *Shard) RunTask(task string) (string, string, error) {
	var outputLen C.size_t = C.sizeof_char * 1024 * 1024
	output := C.malloc(outputLen)
	defer C.free(unsafe.Pointer(output))

	var outputTaskType C.int

	errCode := C.knd_shard_run_task(s.shard, C.CString(task), C.size_t(len(task)), (*C.char)(output), &outputLen)
	if errCode != C.int(0) {
		return "", "", errors.New("could not create shard struct")
	}
	return C.GoStringN((*C.char)(output), C.int(outputLen)), taskTypeToStr(outputTaskType), nil
}
