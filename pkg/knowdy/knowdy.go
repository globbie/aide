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
)

type Shard struct {
	shard   *C.struct_kndShard
	workers chan *C.struct_kndTask
}

func New(conf string, concurrencyFactor int) (*Shard, error) {
	var shard *C.struct_kndShard = nil

	log.Println(conf)

	errCode := C.knd_shard_new((**C.struct_kndShard)(&shard), C.CString(conf), C.size_t(len(conf)))
	if errCode != C.int(0) {
		return nil, errors.New("could not create shard struct")
	}

	proc := Shard{
		shard:   shard,
		workers: make(chan *C.struct_kndTask, concurrencyFactor),
	}

	for i := 0; i < concurrencyFactor; i++ {
		var task *C.struct_kndTask
		errCode := C.knd_task_new(shard, nil, C.int(i), &task)
		if errCode != C.int(0) {
			// todo(n.rodionov): call destructor
			return nil, errors.New("could not create kndTask")
		}
		proc.workers <- task
	}
	return &proc, nil
}

func (s *Shard) Del() error {
	C.kndShard_del__(s.shard)
	return nil
}

func taskTypeToStr(v C.int) string {
	switch v {
	case C.KND_GET_STATE:
		return "get"
	case C.KND_SELECT_STATE:
		return "select"
	case C.KND_UPDATE_STATE:
		return "update"
	default:
		return "unknown"
	}
}

func (s *Shard) RunTask(task string) (string, string, error) {
	worker := <-s.workers
	defer func() { s.workers <- worker }()

	var ctx C.struct_kndTaskContext
        worker.ctx = &ctx
        C.knd_task_reset(worker)

        errCode := C.knd_task_run(worker, C.CString(task), C.size_t(len(task)))
	if errCode != C.int(0) {
		return "", "", errors.New("task execution failed")
	}
	return C.GoStringN((*C.char)(worker.output), C.int(worker.output_size)), taskTypeToStr(C.int(0)), nil
}
