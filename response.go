package stratum

import (
	"encoding/json"
)

type Response struct {
	MessageID interface{}            `json:"id"`
	Result    map[string]interface{} `json:"result"`
	Error     *StratumError          `json:"error"`
}

func (r *Response) String() string {
	b, _ := json.Marshal(r)
	return string(b)
}

// OkResponse generates a response with the following format:
// {"id": "<request.MessageID>", "error": null, "result": {"status": "OK"}}
func OkResponse(r *Request) (*Response, error) {
	return &Response{
		r.MessageID,
		map[string]interface{}{
			"status": "OK",
		},
		nil,
	}, nil
}
