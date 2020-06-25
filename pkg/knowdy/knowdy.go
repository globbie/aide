package knowdy

//#cgo CFLAGS: -I${SRCDIR}/knowdy/include
//#cgo CFLAGS: -I${SRCDIR}/knowdy/libs/gsl-parser/include
//#cgo LDFLAGS: ${SRCDIR}/knowdy/build/lib/libknowdy_static.a
//#cgo LDFLAGS: ${SRCDIR}/knowdy/build/libs/gsl-parser/lib/libgsl-parser_static.a
//#include <knd_shard.h>
//#include <knd_task.h>
// static void kndShard_del__(struct kndShard *shard)
// {
//     if (shard) {
//         knd_shard_del(shard);
//     }
// }
import "C"

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"
	"unsafe"
)

type Shard struct {
	shard      *C.struct_kndShard
	KnowDBAddress string
	LingProcAddress string
	workers    chan *C.struct_kndTask
}

type Message struct {
	Sid       string   `json:"sid,omitempty"`
	Tid       string   `json:"tid,omitempty"`
	Discourse string   `json:"discourse,omitempty"`
	Restate   string   `json:"restate,omitempty"`
	Body      string   `json:"body,omitempty"`
	Resources []string `json:"resources,omitempty"`
	Menu      []string `json:"menu,omitempty"`
}

func New(conf string, KnowDBAddress string,  LingProcAddress string, concurrencyFactor int) (*Shard, error) {
	var shard *C.struct_kndShard = nil
	errCode := C.knd_shard_new((**C.struct_kndShard)(&shard), C.CString(conf), C.size_t(len(conf)))
	if errCode != C.int(0) {
		return nil, errors.New("failed to create a Shard struct")
	}

	s := Shard{
		shard:         shard,
		KnowDBAddress: KnowDBAddress,
		LingProcAddress: LingProcAddress,
		workers:    make(chan *C.struct_kndTask, concurrencyFactor),
	}

	for i := 0; i < concurrencyFactor; i++ {
		var task *C.struct_kndTask
		errCode := C.knd_task_new(shard, nil, C.int(i), &task)
		if errCode != C.int(0) {
			// todo(n.rodionov): call destructor
			return nil, errors.New("could not create kndTask")
		}
		s.workers <- task
	}
	return &s, nil
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
	case C.KND_COMMIT_STATE:
		return "commit"
	default:
		return "unknown"
	}
}

func (s *Shard) RunTask(task string, TaskLen int) (string, string, error) {
	worker := <-s.workers
	defer func() { s.workers <- worker }()

	var ctx C.struct_kndTaskContext
	worker.ctx = &ctx
	C.knd_task_reset(worker)

	cs := C.CString(task)
	defer C.free(unsafe.Pointer(cs))

	errCode := C.knd_task_run(worker, cs, C.size_t(TaskLen))
	if errCode != C.int(0) {
		return "", "", errors.New("task execution failed")
	}

	// check if we need to write to the master node
        switch C.int(ctx.phase) {
	case C.KND_CONFIRM_COMMIT:
		reply, err := s.SendMasterTask(C.GoStringN((*C.char)(worker.output), C.int(worker.output_size)))
		return reply, "commit", err
	default:
		return C.GoStringN((*C.char)(worker.output), C.int(worker.output_size)), taskTypeToStr(C.int(0)), nil
	}
}

func (s *Shard) SendMasterTask(GSL string) (string, error) {
	u := url.URL{Scheme: "http", Host: s.KnowDBAddress, Path: "/gsl"}
	var netClient = &http.Client{
		Timeout: time.Second * 7,
	}

	resp, err := netClient.Post(u.String(), "text/plain; charset=utf-8", bytes.NewBuffer([]byte(GSL)))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	return string(body), nil
}


func (s *Shard) ProcessMsg(sid string, tid string, msg string, lang string) (string, string, error) {
	graph, err := s.DecodeText(msg, lang)
	if err != nil {
		return "", "", errors.New("text decoding failed :: " + err.Error())
        }

	if graph == "{}" {
		return "", "", errors.New("empty graph :: " + err.Error())
        }

        // parse GSL, build msg tree

        // decide what action is needed

        // exec if it's a lightweight / no cost task
        // all heavy / complex / costly  tasks require prior approval from the User
        // these are started from the /gsl endpoint only

        // confirm desired task, send task restatement + quick results if any

        // send task report

        // reply, err := s.EncodeText(graph, lang)
	// if err != nil {
	// 	return "", "", errors.New("text encoding failed :: " + err.Error())
	//}

	reply := Message{sid, tid, "stm", "-- restate --", "-- reply --", []string{"a","b","c"}, []string{"one","two","three"}}
	b, err := json.Marshal(reply)
	return string(b), "msg", nil
}
