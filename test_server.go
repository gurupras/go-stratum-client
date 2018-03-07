package stratum

import (
	"bufio"
	"encoding/json"
	"math/rand"
	"net"
	"strings"

	stoppablenetlistener "github.com/gurupras/go-stoppable-net-listener"
	log "github.com/sirupsen/logrus"
)

type TestServer struct {
	*stoppablenetlistener.StoppableNetListener
	RequestChan chan *ClientRequest
	EventChan   chan interface{}
	jobCounter  int
}

type TsErrorEvent struct {
	*ClientRequest
	error
}

type TsMessageEvent struct {
	Method string
	*ClientRequest
	DefaultResponse *Response
}

type ClientRequest struct {
	Conn    net.Conn
	Request *Request
}

var (
	AUTH_RESPONSE_STR_1    = `{"id":1,"jsonrpc":"2.0","error":null,"result":{"id":"8bc409ea-7ca7-4073-a596-31b1c8fb9335","job":{"blob":"0101c1ef8dd405b9a2d3278fbc35ba876422c82a94dd7befec695f42848d55cab06a96e31a34b300000000b1940501101bd2f22938ce55f25ddf3c584ab6915453073f3c09a5967df6966204","job_id":"Js5ps3OcKxJUCiEjtIz54ImuNMmA","target":"b88d0600","id":"8bc409ea-7ca7-4073-a596-31b1c8fb9335"},"status":"OK"}}` + "\n"
	AUTH_RESPONSE_STR_2    = `{"id":1,"jsonrpc":"2.0","error":null,"result":{"id":"8bc409ea-7ca7-4073-a596-31b1c8fb9335","job":{"blob":"0505efcfdccb0506180897d587b02f9c97037e66ea638990b2b3a0efab7bab0bff4e3f3dfe1c7d00000000a6788e66eb9b82325f95fc7a2007d3fed7152a3590366cc2a9577dcadf3544a804","job_id":"Js5ps3OcKxJUCiEjtIz54ImuNMmA","target":"e4a63d00","id":"8bc409eb-7ca7-4073-a596-31b1c8fb9335"},"status":"OK"}}` + "\n"
	AUTH_RESPONSE_RESULT_2 = "960A7A3A1826B0AA70E8043FFE7B9E23EE2E028BBA75F3D7557CCDFF9C7F1A00"
	AUTH_RESPONSE_STR_3    = `{"id":1,"jsonrpc":"2.0","error":null,"result":{"id":"8bc409ea-7ca7-4073-a596-31b1c8fb9335","job":{"blob":"0707dde5cdd4058e415b279f8e448abc8cf5c97e5768770ad1f2699f932be50511a0925afa9df5000000007d39a83b02d50b39b6ee526555b9bef8d249e03e5871cf3e7a07fba548db00f303","job_id":"K8itLau43TcUF0oqQiq3P3rwuWRZ","target":"5c351400","id":"bd27ab2c-8b59-4486-94f2-fd0b49d173a0"},"status":"OK"}}` + "\n"
	AUTH_RESPONSE_RESULT_3 = `8dac154677f0b053b4fdf2a18fba93e45a0009805a37cd108f0fcee00b0d0600`
	AUTH_RESPONSE_STR_4    = `{"id":1,"jsonrpc":"2.0","error":null,"result":{"id":"73292077-4cb3-4d26-80ec-93d3805c448f","job":{"blob":"0707efeccdd405566a7baee167953d5d3754d82087011228d2bdb36ac0b2798ebb8a264ada03aa00000000041a4cfc113c9be9ce1d7214298837b250a5ea70396dd166681d6dbaa1eb9b2703","job_id":"FF0V3jespRJxKE+aNV/rAFXL6YXu","target":"9bc42000","id":"73292077-4cb3-4d26-80ec-93d3805c448f"},"status":"OK"}}` + "\n"
	AUTH_RESPONSE_RESULT_4 = `46cfea7d7afe4739d517783e3b4b78a48fe173288cd2926a774cc71c94881b00`
	TEST_JOB_STR_1         = `{"jsonrpc":"2.0","method":"job","params":{"blob":"0101dab597d4059fdcc43a65bca7d58238708e97dbe59f21030314c55278c42cbb9ae13ac2e44b00000000b0d68bd268662790c0aae0e79bbdd6c4fd6dabf11485415239930936708e38df07","job_id":"jhUv6SY9RB0Pv+QzyfoZ9sg0Yg1d","target":"877d0200","id":"7ec63ee3-21ae-45ee-abd7-fc44c01508e7"}}` + "\n"
	TEST_JOB_STR_2         = `{"jsonrpc":"2.0","method":"job","params":{"blob":"0101c08affd405a2c58d548dc8b0ad847ef2f9714c55c23f6071256529e6573cda8260d4dc674b000000004485013199e5a45b6873d8dbca3c410d634210c8ff08cc5dd3aae805c85964b801","job_id":"2RnPDwXHTUl5yaCyvFaAgbnJ5MO7","target":"9e0f0800","id":"56bc1d76-0a4a-485a-b77f-c044af2e6df4"}}` + "\n"
	TEST_JOB_STR_3         = `{"jsonrpc":"2.0","method":"job","params":{"blob":"0101c08affd405a2c58d548dc8b0ad847ef2f9714c55c23f6071256529e6573cda8260d4dc674b000000004761695040784182c8a14a3dd204070f23704009e6045415315159e83e4b147b01","job_id":"P7FkijwsW3aQEYobjOHSg1seWFU8","target":"134e0100","id":"e5cf2a03-5072-43b4-b715-5ae94ef1e3bb"}}` + "\n"
	TEST_JOB_STR_4         = `{"jsonrpc":"2.0","method":"job","params":{"blob":"0101c08affd405a2c58d548dc8b0ad847ef2f9714c55c23f6071256529e6573cda8260d4dc674b00000000084ded521877d16cd47290ca02edac705302c367583dc3a71de3a245e76298f501","job_id":"s2reLql9Nqek+80xMNlz8DSwV8pw","target":"df4e0100","id":"e5cf2a03-5072-43b4-b715-5ae94ef1e3bb"}}` + "\n"
	TEST_JOB_STR_5         = `{"jsonrpc":"2.0","method":"job","params":{"blob":"0101c08affd405a2c58d548dc8b0ad847ef2f9714c55c23f6071256529e6573cda8260d4dc674b000000003226efc9748eba339df68d8545bcda69734cc5f2b97860c025f5543e7eaa764b01","job_id":"8A4/5aRUJLkUlyZCCU+yoKVMAS3q","target":"08080800","id":"4aac1d76-0a4a-485a-b77f-c044af2e6df4"}}` + "\n"
	RESULT_JOB_STR_5       = `39916040d5d411d5edcb1a1f90cf133b6dc7c0be05c39b526c3312cadcd40000`
	TEST_JOB_STR           = TEST_JOB_STR_5
	RESULT_JOB_STR         = RESULT_JOB_STR_5

	TestAuthResponses = []string{AUTH_RESPONSE_STR_1, AUTH_RESPONSE_STR_2, AUTH_RESPONSE_STR_3, AUTH_RESPONSE_STR_4}
	TestJobs          = []string{TEST_JOB_STR_1, TEST_JOB_STR_2, TEST_JOB_STR_3, TEST_JOB_STR_4, TEST_JOB_STR_5}
)

// NewTestServer creates a new test server on the specified port.
// Throws an error if it was unable to bind to the specified port.
func NewTestServer(port int) (*TestServer, error) {
	snl, err := stoppablenetlistener.New(port)
	if err != nil {
		return nil, err
	}
	log.Infof("Listening on %v", snl.Addr())
	ts := &TestServer{
		snl,
		make(chan *ClientRequest),
		make(chan interface{}),
		0,
	}
	go func() {
		for {
			conn, err := snl.Accept()
			if err != nil {
				// log.Errorf("Failed to accept connection! Terminating test-server: %v", err)
				break
			}
			log.Infof("Received connection from '%v'", conn.RemoteAddr())
			go ts.handleConnection(conn)
		}
	}()
	return ts, nil
}

func (ts *TestServer) handleConnection(conn net.Conn) {
	log.Infof("Starting handleConnection for conn: %v", conn.RemoteAddr())
	reader := bufio.NewReader(conn)
	for {
		msg, err := reader.ReadString('\n')
		msg = strings.TrimSpace(msg)
		if err != nil {
			log.Infof("Breaking connection from IP '%v': %v", conn.RemoteAddr(), err)
			break
		}
		// log.Infof("Received message from IP: %v: %v", conn.RemoteAddr(), msg)
		var request Request
		if err := json.Unmarshal([]byte(msg), &request); err != nil {
			log.Errorf("Message not in JSON format: '%s': %v", msg, err)
		} else {
			log.Infof("Sending request down RequestChan: %v", request)
			ts.RequestChan <- &ClientRequest{
				conn,
				&request,
			}
		}
	}
}

func (ts *TestServer) defaultHandler() {
	for clientRequest := range ts.RequestChan {
		var err error
		var response *Response
		log.Infof("RequestChan: Received message: %v", clientRequest.Request)
		switch clientRequest.Request.RemoteMethod {
		case "login":
			if response, err = ts.RandomAuthResponse(); err != nil {
				break
			}
		case "submit":
			if response, err = OkResponse(clientRequest.Request); err != nil {
				break
			}
		case "keepalived":
			log.Infof("Handing keepalive")
			id := clientRequest.Request.MessageID
			response = &Response{
				id,
				map[string]interface{}{
					"status": "KEEPALIVED",
				},
				nil,
			}
		}
		if err != nil {
			log.Infof("Sending error down eventChan")
			ts.EventChan <- &TsErrorEvent{
				clientRequest,
				err,
			}
		} else {
			log.Infof("Sending message down eventChan")
			ts.EventChan <- &TsMessageEvent{
				clientRequest.Request.RemoteMethod,
				clientRequest,
				response,
			}
		}
	}
	panic("This goroutine must not terimate")
}

// RandomAuthResponse sends back one of the hard-coded response strings
// consisting of a successful result and a job
func (ts *TestServer) RandomAuthResponse() (*Response, error) {
	var r Response

	randomInt := rand.Intn(len(TestAuthResponses))
	responseBytes := []byte(TestAuthResponses[randomInt])

	if err := json.Unmarshal(responseBytes, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// RandomJob doesn't really generate random jobs. Has a list of jobs
// that it iterates over in order
func (ts *TestServer) RandomJob() (*Request, error) {
	var r Request

	jobBytes := []byte(TestJobs[ts.jobCounter%len(TestJobs)])
	ts.jobCounter++
	if err := json.Unmarshal(jobBytes, &r); err != nil {
		return nil, err
	}
	return &r, nil
}
