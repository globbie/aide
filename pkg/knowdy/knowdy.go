package knowdy

//#cgo CFLAGS: -I${SRCDIR}/knowdy/include
//#cgo CFLAGS: -I${SRCDIR}/knowdy/libs/gsl-parser/include
//#cgo LDFLAGS: ${SRCDIR}/knowdy/build/lib/libknowdy_static.a
//#cgo LDFLAGS: ${SRCDIR}/knowdy/build/libs/gsl-parser/lib/libgsl-parser_static.a
//#include <knd_shard.h>
//#include <knd_task.h>
//#include <knd_output.h>
//#include <knd_text.h>
// static void kndShard_del__(struct kndShard *shard)
// {
//     if (shard) {
//         knd_shard_del(shard);
//     }
// }
import "C"

import (
	"bytes"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unsafe"
	"github.com/dgrijalva/jwt-go"
	"github.com/globbie/aide/pkg/session"
)

type KnowdyClaims struct {
    UserId    string `json:"uid"`
    ShardId   string `json:"shard"`
    jwt.StandardClaims
}

type MenuOption struct {
	Id       string              `json:"opt,omitempty"`
	Title    map[string]string   `json:"title,omitempty"`
}

type GeoTag struct {
	Id       string              `json:"id,omitempty"`
	Lat      float64             `json:"lat,omitempty"`
	Lng      float64             `json:"lng,omitempty"`
	Title    map[string]string   `json:"title,omitempty"`
}

type ScriptPhase struct {
	Id       string              `json:"id,omitempty"`
	Body     map[string]string   `json:"body,omitempty"`
	Quest    map[string]string   `json:"quest,omitempty"`
	Menu     []MenuOption        `json:"menu,omitempty"`
	Resources[]Resource          `json:"resources,omitempty"`
	GeoTags[]GeoTag              `json:"geotags,omitempty"`
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

type ShardInfo struct {
	Name          string               `json:"name"`
	MaxCapacity   int
	Capacity      int
}

type Shard struct {
	shard               *C.struct_kndShard
	Name                string
	ServiceDomain       string
	KnowdyAddress       string
	KnowdyServiceName   string
	LingProcAddress     string
	workers             chan *C.struct_kndTask
	PeerShards          []ShardInfo
	Resources           map[string]Resource
	Scripts             map[string]Script
	LangCaches          []LangCache
	MsgIdx              map[string][]MsgInterp
}

type Resource struct {
	Id       string              `json:"id"`
	ImgId    string              `json:"img,omitempty"`
	Title    map[string]string   `json:"title,omitempty"`
	Body     map[string]string   `json:"body,omitempty"`
}

type Message struct {
	ChatSession  *session.ChatSession     `json:"chatsession,omitempty"`
	Ctx       string              `json:"ctx,omitempty"`
	Discourse string              `json:"discourse,omitempty"`
	Lang      string              `schema:"lang" json:"lang,omitempty"`
	Subj      map[string]string   `json:"subj,omitempty"`
	Input     string              `schema:"t,required"`
	Body      map[string]string   `json:"body,omitempty"`
	Restate   map[string]string   `json:"restate,omitempty"`
	Interp    *json.RawMessage    `json:"interp,omitempty"`
	Resources []Resource          `json:"resources,omitempty"`
	GeoTags   []GeoTag            `json:"geotags,omitempty"`
	Quest     map[string]string   `json:"quest,omitempty"`
	Menu      []MenuOption        `json:"menu,omitempty"`
}

var (
	MaxResources     = 7
	DBCacheFilename    = "/etc/aide/dbcache.json"
	MsgCacheFilename    = "/etc/aide/msgcache.json"
)

func New(conf string, KnowdyAddress string,  KnowdyServiceName string, LingProcAddress string, ServiceDomain string, PeerShards []string, concurrencyFactor int) (*Shard, error) {
	var shard *C.struct_kndShard = nil
	errCode := C.knd_shard_new((**C.struct_kndShard)(&shard), C.CString(conf), C.size_t(len(conf)))
	if errCode != C.int(0) {
		return nil, errors.New("failed to create a Shard struct")
	}

	s := Shard{
		shard:         shard,
		ServiceDomain: ServiceDomain,
		KnowdyAddress: KnowdyAddress,
		KnowdyServiceName: KnowdyServiceName,
		LingProcAddress: LingProcAddress,
		workers:    make(chan *C.struct_kndTask, concurrencyFactor),
	}

	for _, element := range PeerShards {
		si := ShardInfo{
			Name: element,
		}
		s.PeerShards = append(s.PeerShards, si)
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
		// log.Println("JSON DB: ", err.Error())
		return errors.New("failed to read json db cache")
	}
	err = json.Unmarshal(CacheBytes, &s.Scripts)
	if err != nil {
		// log.Println("Unmarshal: ", err.Error())
		return errors.New("failed to read json script db cache")
	}
	// script := s.Scripts["explore"]
	// phase := script.ScriptPhases["init"]
	// b, _ := json.Marshal(phase)
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

	/*for key, interps := range s.MsgIdx {
		log.Println("Msg: ", key, " val:", interps)
		if key == strings.ToUpper("Where do I start") {
			for _, interp := range interps {
				log.Println("Ctx: ", interp.ScriptCtx.Id, " React:", interp.ScriptReact.Id)
				
			}
		}
	}*/

	// reply, _ := s.CacheLookup("123", "init", "Where do I start", "en")
	// log.Println("Reply: ", reply)
	
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
		reply := C.GoStringN((*C.char)(worker.output), C.int(worker.output_size))
		return reply, "", errors.New("task execution failed")
	}

	// check if we need to write to the authority node
        switch C.int(ctx.phase) {
	case C.KND_CONFIRM_COMMIT:
		reply, err := s.ApplyCommit(s.KnowdyServiceName, C.GoStringN((*C.char)(worker.output), C.int(worker.output_size)))
		return reply, "commit", err
	default:
		return C.GoStringN((*C.char)(worker.output), C.int(worker.output_size)), taskTypeToStr(C.int(0)), nil
	}
}

func (s *Shard) ApplyCommit(Address string, GSL string) (string, error) {
	u := url.URL{Scheme: "http", Host: Address, Path: "/gsl"}
	var netClient = &http.Client{
		Timeout: time.Second * 7,
	}
	resp, err := netClient.Post(u.String(), "text/plain; charset=utf-8", bytes.NewBuffer([]byte(GSL)))
	if err != nil {
		log.Println("-- network failure: ", err.Error())
		return "", err
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	if (resp.StatusCode != 200) {
		return string(body), errors.New("failed to apply a commit")
	}
	return string(body), nil
}

func buildMsgReply(ses *session.ChatSession, tid string, ctx string, phase ScriptPhase, lang string) (string, error) {
	//var body strings.Builder
	// body.WriteString("--")
	// body.String()
	log.Println(ses)
	
	reply := Message{nil, ctx, "stm", lang,
		map[string]string{"en":"Reply"}, "", phase.Body, map[string]string{"en":"-- restate --"}, nil,
		phase.Resources, phase.GeoTags, phase.Quest, phase.Menu}

	b, _ := json.Marshal(reply)
	return string(b), nil
}

func (s *Shard) CacheLookup(ses *session.ChatSession, ctx string, msg string, lang string) (string, error) {
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
		return buildMsgReply(ses, script.Id, interp.ScriptReact.Id, script.ScriptPhases["init"], lang)
	}
	return "", errors.New("no valid interp found")
}

func (s *Shard) ProcessMsg(msg *Message) (string, error) {
	msg.Lang = "en" // default lang
	if len(msg.ChatSession.Langs) > 0 {
		msg.Lang = msg.ChatSession.Langs[0].String()
		i := strings.Index(msg.Lang, "-")
		if i != -1 {
			msg.Lang = msg.Lang[:i]
		}
	}
	reply, err := s.CacheLookup(msg.ChatSession, msg.Ctx, msg.Input, msg.Lang)
	if err == nil {
		return reply, nil
        }
	reply, msg.Discourse, err = s.DecodeText(msg.Input, msg.Lang)
	if err != nil {
		return "", errors.New("text decoding failed :: " + err.Error())
        }
	// msg.Body = make(map[string]string)
	// msg.Restate = make(map[string]string)
	// msg.Body[msg.Lang] = reply
	// msg.Restate[msg.Lang] = reply

	{
		json_interp_str, err := s.BuildJSON(reply, msg.Lang)
		if err != nil {
			return "", errors.New("JSON encoding failed :: " + err.Error())
		}
		log.Println(json_interp_str)
		rawJSON := json.RawMessage(json_interp_str)
		msg.Interp = &rawJSON
	}
        // decide what action is needed
	switch msg.Discourse {
	case "stm":
		break
		// return s.CommitStatement(msg)
	case "query":
		break
		// return s.RunQuery(msg)
	case "theme":
		break
		// return s.CommitStatement(msg)
		// return s.RunQuery(msg)
	default:
		break
	}

        // exec if it's a lightweight / no cost task
        // all heavy / complex / costly  tasks require prior approval from the User
        // these are started from the /gsl endpoint only

        // confirm desired task, send task restatement + quick results if any

        // send task report

        // reply, err := s.EncodeText(graph, lang)
	// if err != nil {
	// 	return "", "", errors.New("text encoding failed :: " + err.Error())
	//}

	b, err := json.Marshal(msg)
	return string(b), nil
}

func (s *Shard) CommitStatement(msg *Message) (string, error) {
	var gsl strings.Builder
	gsl.WriteString("{task{user " + msg.ChatSession.UserId)
	gsl.WriteString("{repo ~{class Message{!inst _{body{_t")
	gsl.WriteString(msg.Body[msg.Lang])
	gsl.WriteString("}}}}}}}")

	log.Println(">> stm commit in progress: ", gsl.String())
	report, err := s.ApplyCommit(s.KnowdyAddress, gsl.String())
	if err != nil {
		return "", errors.New("failed to save a user message")
	}
	log.Println("== commit report:", report)

	b, _ := json.Marshal(msg)
	return string(b), nil
}

func (s *Shard) RunQuery(msg *Message) (string, error) {
	log.Println(".. Session ", msg.ChatSession, " run query: ", msg.Restate[msg.Lang])
	
	b, _ := json.Marshal(msg)
	return string(b), nil
}

func (s *Shard) CreateChatSession(ses *session.ChatSession, signKey *rsa.PrivateKey) (string, []*http.Cookie, error) {
	var si *ShardInfo = nil
	// select public shard to host a new session
	// TODO: check current capacity
	for _, elem := range s.PeerShards {
		if elem.Name == "public" {
			si = &elem
			break
		}
	}
	if si == nil {
		return "", nil, errors.New("failed to find a public peer shard")
	}
	ses.ShardId = si.Name

	gsl := bytes.Buffer{}
	gsl.WriteString("{task {class User {!inst _")
	if ses.UserAgent != "" {
		gsl.WriteString("[soft{" + ses.UserAgent +"}]")
	}
	if ses.UserIP != "" {
		gsl.WriteString("[ip-allow{" + ses.UserIP +"}]")
	}
	if len(ses.Langs) > 0 {
		gsl.WriteString("[lang")
		for _, langtag := range ses.Langs {
			gsl.WriteString("{" + langtag.String() +"}")
		}
		gsl.WriteString("]")
	}
	gsl.WriteString("}}}")

	// register new user
	{
		// var addr = s.KnowdyServiceName + "-" + si.Name
		report, err := s.ApplyCommit(s.KnowdyAddress, gsl.String())
		if err != nil {
			log.Println("failed to register a user:" + report)
			return "", nil, errors.New("failed to register a user")
		}
		log.Println(report)
		// TMP
		pref := "{class User{!inst "
		pref_size := len(pref)
		i := strings.Index(report, pref)
		if i == -1 {
			return "", nil, errors.New("failed to extract uid: no user inst")
		}
		remainder := report[i + pref_size:]
		idbuf := bytes.Buffer{}
		for _, c := range remainder {
			if c == '{' || c == '[' || c == ' '{
				break
			}
			idbuf.WriteByte(byte(c))
		}
		if idbuf.Len() == 0 {
			return "", nil, errors.New("failed to extract uid")
		}
		ses.UserId = idbuf.String()
		log.Println("== uid:", ses.UserId)
	}
	// build access token
	token, err := session.IssueAccessToken(ses, signKey, 64)
	if err != nil {
		return "", nil, errors.New("failed to issue SID token")
	}
	var cookie *http.Cookie
	var cookies []*http.Cookie
	cookie, err = session.BuildSessionCookie("sid", token, s.ServiceDomain)
	cookies = append(cookies, cookie)

	// TODO: build initial greetings, menu options etc.
	reply := "{\"sid\":\"" + token + "\",\"uid\":\"" + ses.UserId + "\"}"
	return reply, cookies, nil
}

func (s *Shard) BuildJSON(Text string, Lang string) (string, error) {
	worker := <-s.workers
	defer func() { s.workers <- worker }()

	var ctx C.struct_kndTaskContext
	worker.ctx = &ctx
	C.knd_task_reset(worker)

	t := C.CString(Text)
	defer C.free(unsafe.Pointer(t))

	lang := C.CString(Lang)
	defer C.free(unsafe.Pointer(lang))

	ctx.locale_size = C.size_t(len(Lang))
	C.memcpy(unsafe.Pointer(&ctx.locale[0]), unsafe.Pointer(lang), ctx.locale_size)

	errCode := C.knd_text_build_JSON(t, C.size_t(len(Text)), worker)
	if errCode != C.int(0) {
		msg := C.GoStringN((*C.char)(worker.log.buf), C.int(worker.log.buf_size))
		log.Println(msg)
		return "", errors.New("JSON text build failed")
	}

	return C.GoStringN((*C.char)(worker.output), C.int(worker.output_size)), nil
}
