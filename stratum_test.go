package stratum

import (
	"fmt"
	"io/ioutil"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v2"
)

var testConfig map[string]interface{}

func connect(sc *StratumContext) error {
	err := sc.Connect(testConfig["pool"].(string))
	if err != nil {
		log.Debugf("Connected to pool..")
	}
	return err
}

func TestConnect(t *testing.T) {
	require := require.New(t)

	sc := New()
	err := connect(sc)
	require.Nil(err)
}

func TestBadAuthorize(t *testing.T) {
	require := require.New(t)

	sc := New()
	err := connect(sc)
	require.Nil(err)

	err = sc.Authorize("", testConfig["pass"].(string))
	require.NotNil(err)
}

func TestAuthorize(t *testing.T) {
	require := require.New(t)

	sc := New()
	err := connect(sc)
	require.Nil(err)

	wg := sync.WaitGroup{}
	wg.Add(1)

	workChan := make(chan *Work)
	sc.RegisterWorkListener(workChan)

	go func() {
		defer wg.Done()
		for _ = range workChan {
			break
		}
	}()

	err = sc.Authorize(testConfig["username"].(string), testConfig["pass"].(string))
	require.Nil(err)
	wg.Wait()
}

func TestGetJob(t *testing.T) {
	t.Skip("Cannot arbitrarily call sc.GetJob()")
	require := require.New(t)

	sc := New()
	err := connect(sc)
	require.Nil(err)

	wg := sync.WaitGroup{}
	wg.Add(2)

	workChan := make(chan *Work)
	sc.RegisterWorkListener(workChan)

	go func() {
		for _ = range workChan {
			log.Debugf("Calling wg.Done()")
			wg.Done()
		}
	}()

	err = sc.Authorize(testConfig["username"].(string), testConfig["pass"].(string))
	require.Nil(err)

	err = sc.GetJob()
	require.Nil(err)
	wg.Wait()
}

func TestReconnect(t *testing.T) {
	require := require.New(t)

	server, err := NewTestServer(7223)
	require.Nil(err)

	wg := sync.WaitGroup{}
	wg.Add(5)

	go func() {
		for obj := range server.EventChan {
			log.Infof("test: Got object: %t", obj)
			switch evt := obj.(type) {
			case *TsErrorEvent:
				require.Fail(fmt.Sprintf("Unexpected failure on method=%v: %v", evt.ClientRequest.Request.RemoteMethod, evt.Error()))
			case *TsMessageEvent:
				log.Debugf("Message event")
				switch evt.Method {
				case "login":
					log.Infof("login event")
					_, err = evt.ClientRequest.Conn.Write([]byte(evt.DefaultResponse.String() + "\n"))
					require.Nil(err)
					wg.Done()
				case "submit":
					log.Infof("submit event")
					_, err = evt.ClientRequest.Conn.Write([]byte(evt.DefaultResponse.String() + "\n"))
					require.Nil(err)
					request, err := server.RandomJob()
					require.Nil(err)
					requestStr, err := request.JsonRPCString()
					require.Nil(err)
					_, err = evt.ClientRequest.Conn.Write([]byte(requestStr))
					require.Nil(err)
				case "keepalived":
					log.Infof("keepalived event")
					// Write partial message and close connection
					_, err = evt.ClientRequest.Conn.Write([]byte(evt.DefaultResponse.String()[:10] + "\n"))
					require.Nil(err)
					evt.ClientRequest.Conn.Close()
				default:
					require.Fail(fmt.Sprintf("Unknown remoteMethod: %v", evt.Method))
				}
			default:
				require.Fail("Received unknown event: %t", evt)
			}
		}
	}()

	go server.defaultHandler()

	sc := New()
	err = sc.Connect("localhost:7223")
	require.Nil(err)

	workChan := make(chan *Work)
	sc.RegisterWorkListener(workChan)

	go func() {
		for work := range workChan {
			go func(work *Work) {
				time.Sleep(300 * time.Millisecond)
				log.Debugf("Triggering work submission")
				if err := sc.SubmitWork(work, "0"); err != nil {
					log.Warnf("Failed work submission: %v", err)
				}
			}(work)
		}
	}()

	sc.KeepAliveDuration = 1 * time.Second
	err = sc.Authorize(testConfig["username"].(string), testConfig["pass"].(string))
	require.Nil(err)
	wg.Wait()
	server.Close()
}

func TestKeepAlive(t *testing.T) {
	require := require.New(t)

	server, err := NewTestServer(7223)
	require.Nil(err)

	wg := sync.WaitGroup{}
	count := 10
	wg.Add(1)

	go func() {
		defer wg.Done()
		for obj := range server.EventChan {
			switch evt := obj.(type) {
			case *TsErrorEvent:
				require.Fail(fmt.Sprintf("Unexpected failure on method=%v: %v", evt.ClientRequest.Request.RemoteMethod, evt.Error()))
			case *TsMessageEvent:
				switch evt.Method {
				case "login":
					_, err = evt.ClientRequest.Conn.Write([]byte(evt.DefaultResponse.String() + "\n"))
					require.Nil(err)
					wg.Done()
				case "submit":
					_, err = evt.ClientRequest.Conn.Write([]byte(evt.DefaultResponse.String() + "\n"))
					require.Nil(err)
					request, err := server.RandomJob()
					require.Nil(err)
					requestStr, err := request.JsonRPCString()
					require.Nil(err)
					_, err = evt.ClientRequest.Conn.Write([]byte(requestStr))
					require.Nil(err)
				case "keepalived":
					// Write partial message and close connection
					_, err = evt.ClientRequest.Conn.Write([]byte(evt.DefaultResponse.String()[:10] + "\n"))
					require.Nil(err)
					count--
					if count == 0 {
						return
					}
				default:
					require.Fail(fmt.Sprintf("Unknown remoteMethod: %v", evt.Method))
				}
			default:
				require.Fail("Received unknown event: %t", evt)
			}
		}
	}()
	go server.defaultHandler()

	sc := New()
	err = sc.Connect("localhost:7223")
	require.Nil(err)

	workChan := make(chan *Work)
	sc.RegisterWorkListener(workChan)

	go func() {
		for work := range workChan {
			go func(work *Work) {
				time.Sleep(300 * time.Millisecond)
				log.Debugf("Triggering work submission")
				if err := sc.SubmitWork(work, "0"); err != nil {
					log.Warnf("Failed work submission: %v", err)
				}
			}(work)
		}
	}()
	sc.KeepAliveDuration = 100 * time.Millisecond
	err = sc.Authorize(testConfig["username"].(string), testConfig["pass"].(string))
	require.Nil(err)
	wg.Wait()
	server.Close()
}

func TestParallelWrites(t *testing.T) {
	require := require.New(t)

	server, err := NewTestServer(7223)
	require.Nil(err)

	wg := sync.WaitGroup{}
	count := int32(10000 * 10)
	wg.Add(1)

	go func() {
		defer wg.Done()
		for obj := range server.EventChan {
			switch evt := obj.(type) {
			case *TsErrorEvent:
				require.Fail(fmt.Sprintf("Unexpected failure on method=%v: %v", evt.ClientRequest.Request.RemoteMethod, evt.Error()))
			case *TsMessageEvent:
				switch evt.Method {
				case "login":
					_, err = evt.ClientRequest.Conn.Write([]byte(evt.DefaultResponse.String() + "\n"))
					require.Nil(err)
					wg.Done()
				case "submit":
					require.Fail("Unexpectedly received a job submission")
				case "keepalived":
					// Write partial message and close connection
					_, err = evt.ClientRequest.Conn.Write([]byte(evt.DefaultResponse.String()[:10] + "\n"))
					require.Nil(err)
					atomic.AddInt32(&count, -1)
					if count == 0 {
						return
					}
				default:
					require.Fail(fmt.Sprintf("Unknown remoteMethod: %v", evt.Method))
				}
			default:
				require.Fail("Received unknown event: %t", evt)
			}
		}
	}()
	go server.defaultHandler()

	sc := New()
	err = sc.Connect("localhost:7223")
	require.Nil(err)

	workChan := make(chan *Work)
	sc.RegisterWorkListener(workChan)

	go func() {
		for _ = range workChan {
		}
	}()
	sc.KeepAliveDuration = 10 * time.Millisecond
	err = sc.Authorize(testConfig["username"].(string), testConfig["pass"].(string))
	// Start several goroutines that bombard the server with keepalive messages
	for i := 0; i < 1000; i++ {
		go sc.RunKeepAlive()
	}
	require.Nil(err)
	wg.Wait()
	server.Close()
}

func TestMain(m *testing.M) {
	log.SetLevel(log.WarnLevel)

	b, err := ioutil.ReadFile("test-config.yaml")
	if err != nil {
		log.Errorf("No test-config.yaml")
		str := `pool:
username:
pass:
`
		if err := ioutil.WriteFile("test-config.yaml", []byte(str), 0666); err != nil {
			log.Errorf("Failed to create test-config.yaml: %v", err)
		} else {
			log.Infof("Created test-config.yaml..run tests after filling it out")
			os.Exit(-1)
		}
	} else {
		if err := yaml.Unmarshal(b, &testConfig); err != nil {
			log.Fatalf("Failed to unmarshal test-config.yaml: %v", err)
		}
	}
	os.Exit(m.Run())
}
