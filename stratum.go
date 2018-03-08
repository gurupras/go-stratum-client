package stratum

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/fatih/set"
	log "github.com/sirupsen/logrus"
)

var (
	KeepAliveDuration time.Duration = 60 * time.Second
)

type StratumOnWorkHandler func(work *Work)
type StratumContext struct {
	net.Conn
	sync.Mutex
	reader                  *bufio.Reader
	id                      int
	SessionID               string
	KeepAliveDuration       time.Duration
	Work                    *Work
	workListeners           set.Interface
	submitListeners         set.Interface
	responseListeners       set.Interface
	LastSubmittedWork       *Work
	submittedWorkRequestIds set.Interface
	numAcceptedResults      uint64
	numSubmittedResults     uint64
	url                     string
	username                string
	password                string
	connected               bool
	lastReconnectTime       time.Time
	stopChan                chan struct{}
}

func New() *StratumContext {
	sc := &StratumContext{}
	sc.KeepAliveDuration = KeepAliveDuration
	sc.workListeners = set.New()
	sc.submitListeners = set.New()
	sc.responseListeners = set.New()
	sc.submittedWorkRequestIds = set.New()
	sc.stopChan = make(chan struct{})
	return sc
}

func (sc *StratumContext) Connect(addr string) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}

	log.Debugf("Dial success")
	sc.url = addr
	sc.Conn = conn
	sc.reader = bufio.NewReader(conn)
	return nil
}

func (sc *StratumContext) Call(serviceMethod string, args interface{}) (*Request, error) {
	sc.id++

	req := NewRequest(sc.id, serviceMethod, args)
	str, err := req.JsonRPCString()
	if err != nil {
		return nil, err
	}
	// sc.Lock()
	// defer sc.Unlock()
	if _, err := sc.Write([]byte(str)); err != nil {
		return nil, err
	}
	log.Debugf("Sent to server via conn: %v: %v", sc.Conn.LocalAddr(), str)
	return req, nil
}

func (sc *StratumContext) ReadLine() (string, error) {
	line, err := sc.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func (sc *StratumContext) ReadJSON() (map[string]interface{}, error) {
	line, err := sc.reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	var ret map[string]interface{}
	if err = json.Unmarshal([]byte(line), &ret); err != nil {
		return nil, err
	}
	return ret, nil
}

func (sc *StratumContext) ReadResponse() (*Response, error) {
	line, err := sc.ReadLine()
	if err != nil {
		return nil, err
	}
	line = strings.TrimSpace(line)
	log.Debugf("Server sent back: %v", line)
	return ParseResponse([]byte(line))
}

func (sc *StratumContext) Authorize(username, password string) error {
	log.Debugf("Beginning authorize")
	args := make(map[string]interface{})
	args["login"] = username
	args["pass"] = password
	args["agent"] = "go-stratum-client"

	_, err := sc.Call("login", args)
	if err != nil {
		return err
	}

	log.Debugf("Triggered login..awaiting response")
	response, err := sc.ReadResponse()
	if err != nil {
		return err
	}
	if response.Error != nil {
		return response.Error
	}

	log.Debugf("Authorization successful")
	sc.connected = true
	sc.username = username
	sc.password = password

	if sid, ok := response.Result["id"]; !ok {
		return fmt.Errorf("Response did not have a sessionID: %v", response.String())
	} else {
		sc.SessionID = sid.(string)
	}
	work, err := ParseWork(response.Result["job"].(map[string]interface{}))
	if err != nil {
		return err
	}
	sc.NotifyNewWork(work)

	// Handle messages
	go sc.RunHandleMessages()
	// Keep-alive
	go sc.RunKeepAlive()

	return nil
}

func (sc *StratumContext) RunKeepAlive() {
	for {
		select {
		case <-sc.stopChan:
			return
		case <-time.After(sc.KeepAliveDuration):
			args := make(map[string]interface{})
			args["id"] = sc.SessionID
			if _, err := sc.Call("keepalived", args); err != nil {
				log.Errorf("Failed keepalive: %v", err)
			} else {
				log.Debugf("Posted keepalive")
			}
		}
	}
}

func (sc *StratumContext) RunHandleMessages() {
	for sc.connected {
		line, err := sc.ReadLine()
		if err != nil {
			log.Debugf("Failed to read string from stratum: %v", err)
			break
		}
		log.Debugf("Received line from server: %v", line)

		var msg map[string]interface{}
		if err = json.Unmarshal([]byte(line), &msg); err != nil {
			log.Errorf("Failed to unmarshal line into JSON: '%s': %v", line, err)
			break
		}

		id := msg["id"]
		switch id.(type) {
		case uint64, float64:
			// This is a response
			response, err := ParseResponse([]byte(line))
			if err != nil {
				log.Errorf("Failed to parse response from server: %v", err)
				continue
			}
			isError := false
			if response.Result == nil {
				// This is an error
				isError = true
			}
			id := uint64(response.MessageID.(float64))
			if sc.submittedWorkRequestIds.Has(id) {
				if !isError {
					// This is a response from the server signalling that our work has been accepted
					sc.submittedWorkRequestIds.Remove(id)
					sc.numAcceptedResults++
					sc.numSubmittedResults++
					log.Infof("accepted %d/%d", sc.numAcceptedResults, sc.numSubmittedResults)
				} else {
					sc.submittedWorkRequestIds.Remove(id)
					sc.numSubmittedResults++
					log.Errorf("rejected %d/%d: %s", (sc.numSubmittedResults - sc.numAcceptedResults), sc.numSubmittedResults, response.Error.Message)
				}
			} else {
				statusIntf, ok := response.Result["status"]
				if !ok {
					log.Warnf("Server sent back unknown message: %v", response.String())
				} else {
					status := statusIntf.(string)
					switch status {
					case "KEEPALIVED":
						// Nothing to do
					case "OK":
						log.Errorf("Failed to properly mark submitted work as accepted. work ID: %v, message=%s", response.MessageID, response.String())
						log.Errorf("Works: %v", sc.submittedWorkRequestIds.List())
					}
				}
			}
			sc.NotifyResponse(response)
		default:
			// this is a notification
			log.Debugf("Received message from stratum server: %v", msg)
			switch msg["method"].(string) {
			case "job":
				if work, err := ParseWork(msg["params"].(map[string]interface{})); err != nil {
					log.Errorf("Failed to parse job: %v", err)
					continue
				} else {
					sc.NotifyNewWork(work)
				}
			default:
				log.Errorf("Unknown method: %v", msg["method"])
			}
		}
	}
	sc.Reconnect()
}

func (sc *StratumContext) Reconnect() {
	// sc.Lock()
	sc.stopChan <- struct{}{}
	if sc.Conn != nil {
		sc.Close()
		sc.Conn = nil
	}
	// sc.Unlock()
	log.Infof("Reconnecting ...")
	now := time.Now()
	if now.Sub(sc.lastReconnectTime) < 1*time.Second {
		time.Sleep(1 * time.Second) //XXX: Should we sleeping the remaining time?
	}
	if err := sc.Connect(sc.url); err != nil {
		// TODO: We should probably try n-times before crashing
		log.Fatalf("Failled to reconnect to %v: %v", sc.url, err)
	}
	log.Debugf("Connected. Authorizing ...")
	sc.Authorize(sc.username, sc.password)
}

func (sc *StratumContext) SubmitWork(work *Work, hash string) error {
	if work == sc.LastSubmittedWork {
		// log.Warnf("Prevented submission of stale work")
		// return nil
	}
	args := make(map[string]interface{})
	nonceStr, err := BinToHex(work.Data[39:43])
	if err != nil {
		return err
	}
	args["id"] = sc.SessionID
	args["job_id"] = work.JobID
	args["nonce"] = nonceStr
	args["result"] = hash
	if req, err := sc.Call("submit", args); err != nil {
		return err
	} else {
		sc.submittedWorkRequestIds.Add(uint64(req.MessageID.(int)))
		// Successfully submitted result
		log.Debugf("Successfully submitted work result: job=%v result=%v", work.JobID, hash)
		args["work"] = work
		sc.NotifySubmit(args)
		sc.LastSubmittedWork = work
	}
	return nil
}

func (sc *StratumContext) RegisterSubmitListener(sChan chan interface{}) {
	log.Debugf("Registerd stratum.submitListener")
	sc.submitListeners.Add(sChan)
}

func (sc *StratumContext) RegisterWorkListener(workChan chan *Work) {
	log.Debugf("Registerd stratum.workListener")
	sc.workListeners.Add(workChan)
}

func (sc *StratumContext) RegisterResponseListener(rChan chan *Response) {
	log.Debugf("Registerd stratum.responseListener")
	sc.responseListeners.Add(rChan)
}

func (sc *StratumContext) GetJob() error {
	args := make(map[string]interface{})
	args["id"] = sc.SessionID
	_, err := sc.Call("getjob", args)
	return err
}

func ParseResponse(b []byte) (*Response, error) {
	var response Response
	if err := json.Unmarshal(b, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (sc *StratumContext) NotifyNewWork(work *Work) {
	if (sc.Work != nil && strings.Compare(work.JobID, sc.Work.JobID) == 0) || sc.submittedWorkRequestIds.Has(work.JobID) {
		log.Warnf("Duplicate job request. Reconnecting to: %v", sc.url)
		// Just disconnect
		sc.connected = false
		sc.Close()
		return
	}
	log.Infof("\x1B[01;35mnew job\x1B[0m from \x1B[01;37m%v\x1B[0m diff \x1B[01;37m%d \x1B[0m ", sc.RemoteAddr(), int(work.Difficulty))
	sc.Work = work
	for _, obj := range sc.workListeners.List() {
		ch := obj.(chan *Work)
		ch <- work
	}
}

func (sc *StratumContext) NotifySubmit(data interface{}) {
	for _, obj := range sc.submitListeners.List() {
		ch := obj.(chan interface{})
		ch <- data
	}
}

func (sc *StratumContext) NotifyResponse(response *Response) {
	for _, obj := range sc.responseListeners.List() {
		ch := obj.(chan *Response)
		ch <- response
	}
}
