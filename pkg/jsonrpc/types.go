package jsonrpc

import (
	"bytes"
	"encoding/json"
)

type Request struct {
	ID     interface{} `json:"id,omitempty"`
	Method string      `json:"method,omitempty"`
	Params interface{} `json:"params,omitempty"`
}

type Response struct {
	Version string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  interface{}     `json:"result"`
	Error   *Error          `json:"error,omitempty"`
}

type Error struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

var Null = json.RawMessage("null")

const Version = "2.0"

const (
	ErrCodeParse          = -32700
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32601
)

func NewResultResponse(id interface{}, result interface{}) *Response {
	return newResponse(id, result, nil)
}

func NewErrorResponse(id interface{}, error Error) *Response {
	return newResponse(id, nil, &error)
}

func NewErrorStringResponse(id interface{}, code int, msg string) *Response {
	return newResponse(id, nil, &Error{Code: code, Message: msg, Data: nil})
}

func NewParseErrorResponse(err error) *Response {
	return NewErrorResponse(Null, Error{
		Code:    ErrCodeParse,
		Message: "Parse error",
		Data:    err.Error(),
	})
}

func NewMethodNotFoundResponse(id interface{}) *Response {
	return NewErrorResponse(id, Error{
		Code:    ErrCodeMethodNotFound,
		Message: "Method not found",
	})
}

func NewInvalidParamsResponse(id interface{}) *Response {
	return NewErrorResponse(id, Error{
		Code:    ErrCodeInvalidParams,
		Message: "Invalid Params",
	})
}

func newResponse(id interface{}, result interface{}, error *Error) *Response {
	if id == nil {
		return nil
	}
	return &Response{
		Version: Version,
		ID:      encodeResponseID(id),
		Result:  result,
		Error:   error,
	}
}

func encodeResponseID(id interface{}) json.RawMessage {
	if id == nil {
		return Null
	}
	buf, err := json.Marshal(id)
	if err != nil {
		return Null
	}
	return buf
}

func IsBatch(data []byte) bool {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return false
	}
	switch a := tok.(type) {
	case json.Delim:
		return a == '['
	default:
		return false
	}
}

func ParseRequest(data []byte) (reqs []Request, batch bool, err error) {
	if IsBatch(data) {
		var reqs []Request
		if err := json.Unmarshal(data, &reqs); err != nil {
			return nil, false, err
		}
		return reqs, true, nil
	}

	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, false, err
	}
	return []Request{req}, false, nil
}
