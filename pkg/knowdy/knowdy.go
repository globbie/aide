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
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unsafe"
)

type MenuOption struct {
	Id       string              `json:"opt,omitempty"`
	Title    map[string]string   `json:"title,omitempty"`
}

type ScriptPhase struct {
	Id       string              `json:"id,omitempty"`
	Body     map[string]string   `json:"body,omitempty"`
	Quest    map[string]string   `json:"quest,omitempty"`
	Menu     []MenuOption        `json:"menu,omitempty"`
	Resources[]Resource          `json:"resources,omitempty"`
}

type Script struct {
	Id       string                     `json:"id"`
	ScriptPhases map[string]ScriptPhase `json:"phases,omitempty"`
}

type MsgInterp struct {
	ScriptCtx        *ScriptCtx
	ScriptReact      *ScriptReact
}

type ScriptReact struct {
	Id          string     `json:"id"`
	Triggers    []string   `json:"triggers,omitempty"`
}

type ScriptCtx struct {
	Id           string         `json:"id"`
	ScriptReacts  []ScriptReact `json:"scripts,omitempty"`
}

type LangCache struct {
	Id             string               `json:"id"`
	ScriptCtxs     []ScriptCtx          `json:"ctxs,omitempty"`
}

type Shard struct {
	shard           *C.struct_kndShard
	KnowDBAddress   string
	LingProcAddress string
	workers         chan *C.struct_kndTask
	Resources       map[string]Resource
	Scripts         map[string]Script
	LangCaches      []LangCache
	MsgIdx          map[string][]MsgInterp
}

type Resource struct {
	Id       string   `json:"id"`
	ImgId    string   `json:"img,omitempty"`
}

type Message struct {
	Sid       string              `json:"sid,omitempty"`
	Tid       string              `json:"tid,omitempty"`
	Discourse string              `json:"discourse,omitempty"`
	Restate   string              `json:"restate,omitempty"`
	Subj      string              `json:"subj,omitempty"`
	Body      map[string]string   `json:"body,omitempty"`
	Quest     map[string]string   `json:"quest,omitempty"`
	Resources []Resource          `json:"resources,omitempty"`
	Menu      []MenuOption        `json:"menu,omitempty"`
}

var (
	MaxResources     = 7
	DBCacheFilename    = "/etc/aide/dbcache.json"
	MsgCacheFilename    = "/etc/aide/msgcache.json"
)

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

	err := s.PopulateScriptCache(DBCacheFilename)
	if err != nil {
		return nil, errors.New("failed to read json script db cache")
	}
	err = s.PopulateMsgCache(MsgCacheFilename)
	if err != nil {
		return nil, errors.New("failed to read msg cache")
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

func (s *Shard) PopulateScriptCache(Filename string) (error) {
	CacheBytes, err := ioutil.ReadFile(Filename)
	if err != nil {
		log.Println("JSON DB: ", err.Error())
		return errors.New("failed to read json db cache")
	}
	log.Println("JSON DB: ", string(CacheBytes))
	err = json.Unmarshal(CacheBytes, &s.Scripts)
	if err != nil {
		log.Println("Unmarshal: ", err.Error())
		return errors.New("failed to read json script db cache")
	}
	
	script := s.Scripts["sc-explore"]
	phase := script.ScriptPhases["init"]
	b, _ := json.Marshal(phase)
	log.Println("init script phase: ", string(b))
	return nil
}

func (s *Shard) PopulateMsgCache(Filename string) (error) {
	CacheBytes, err := ioutil.ReadFile(Filename)
	if err != nil {
		log.Println("JSON Msg DB: ", err.Error())
		return errors.New("failed to read json msg cache")
	}
	log.Println("Msg DB: ", string(CacheBytes))
	err = json.Unmarshal(CacheBytes, &s.LangCaches)
	if err != nil {
		log.Println("Unmarshal: ", err.Error())
		return errors.New("failed to parse json msg cache")
	}

	s.MsgIdx = make(map[string][]MsgInterp)

	for _, lc := range s.LangCaches {
		for _, ctx := range lc.ScriptCtxs {
			for _, react := range ctx.ScriptReacts {
				for _, trig := range react.Triggers {
					s.RegisterMsg(trig, react, ctx)
				}}}}

	for key, interps := range s.MsgIdx {
		log.Println("Msg: ", key, " val:", interps)
		if key == strings.ToUpper("Where do I start") {
			for _, interp := range interps {
				log.Println("Ctx: ", interp.ScriptCtx.Id, " React:", interp.ScriptReact.Id)
				
			}
		}
	}

	reply, _ := s.CacheLookup("123", "init", "Where do I start", "en")
	log.Println("Reply: ", reply)
	
	return nil
}

func (s *Shard) RegisterMsg(msg string, react ScriptReact, ctx ScriptCtx) (error) {
	uc_msg := strings.ToUpper(msg)
	interps, is_present := s.MsgIdx[uc_msg]
	if !is_present {
		interps = make([]MsgInterp, 0)
	}
	interp := MsgInterp{&ctx, &react}
	s.MsgIdx[uc_msg] = append(interps, interp)
	return nil
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

func buildMsgReply(sid string, ctx string, phase ScriptPhase, lang string) (string, error) {
	//var body strings.Builder
	// body.WriteString("--")
	// body.String()

	reply := Message{sid, ctx, "stm", "Reply", "-- restate --",
		phase.Body, phase.Quest, phase.Resources, phase.Menu}
	
	b, _ := json.Marshal(reply)
	return string(b), nil
}

func (s *Shard) CacheLookup(sid string, ctx string, msg string, lang string) (string, error) {
	k := strings.ToUpper(msg)
	k = strings.TrimSpace(k)

	interps, is_present := s.MsgIdx[k]
	if !is_present {
		return "", errors.New("key not present in cache")
	}

	for _, interp := range interps {
		if ctx != interp.ScriptCtx.Id { continue }

		log.Println("Ctx: ", interp.ScriptCtx.Id, " React:", interp.ScriptReact.Id)

		script, is_present := s.Scripts[interp.ScriptReact.Id]
		if !is_present {
			return "", errors.New("script not found")
		}
		return buildMsgReply(sid, script.Id, script.ScriptPhases["init"], lang)
	}
	return "", errors.New("no valid interp found")
}

func (s *Shard) ProcessMsg(sid string, tid string, msg string, lang string) (string, string, error) {
	reply, err := s.CacheLookup(sid, tid, msg, lang)
	if err == nil {
		return reply, "msg", nil
        }

	_, err = s.DecodeText(msg, lang)
	if err != nil {
		return "", "", errors.New("text decoding failed :: " + err.Error())
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

	m := Message{sid, tid, "stm", "Subj", "-- restate --", nil, nil, nil, nil}
	b, err := json.Marshal(m)
	return string(b), "msg", nil
}
