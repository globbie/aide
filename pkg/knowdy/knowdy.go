package knowdy

// #cgo CFLAGS: -I${SRCDIR}/knowdy/include
// #cgo CFLAGS: -I${SRCDIR}/knowdy/libs/gsl-parser/include
// #cgo LDFLAGS: ${SRCDIR}/knowdy/build/lib/libknowdy_static.a
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
	"github.com/dgrijalva/jwt-go"
	"log"
	"unsafe"
)

type Shard struct {
	shard      *C.struct_kndShard
	gltAddress string
	workers    chan *C.struct_kndTask
}

func New(conf string, gltAddress string, concurrencyFactor int) (*Shard, error) {
	var shard *C.struct_kndShard = nil
	errCode := C.knd_shard_new((**C.struct_kndShard)(&shard), C.CString(conf), C.size_t(len(conf)))
	if errCode != C.int(0) {
		return nil, errors.New("could not create shard struct")
	}

	proc := Shard{
		shard:      shard,
		gltAddress: gltAddress,
		workers:    make(chan *C.struct_kndTask, concurrencyFactor),
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

func (s *Shard) RunTask(task string, task_len int) (string, string, error) {
	worker := <-s.workers
	defer func() { s.workers <- worker }()

	var ctx C.struct_kndTaskContext
	worker.ctx = &ctx
	C.knd_task_reset(worker)

	cs := C.CString(task)
	defer C.free(unsafe.Pointer(cs))

	errCode := C.knd_task_run(worker, cs, C.size_t(task_len))
	if errCode != C.int(0) {
		return "", "", errors.New("task execution failed")
	}
	return C.GoStringN((*C.char)(worker.output), C.int(worker.output_size)), taskTypeToStr(C.int(0)), nil
}

func (s *Shard) ReadMsg(msg string, token *jwt.Token) (string, string, error) {
	claims := token.Claims.(jwt.MapClaims)

	log.Println("MSG:", msg, " from:", claims["email"])

	graph, err := DecodeText(msg, s.gltAddress)
	if err != nil {
		return "", "", errors.New("text decoding failed")
	}
	log.Println("GSL:", graph)

	reply, err := EncodeText(graph, "RU SyNode CS", s.gltAddress)
	if err != nil {
		return "", "", errors.New("text encoding failed")
	}
	log.Println("REPLY:", reply, " err:", err)

	return "OK", "msg", nil
}
