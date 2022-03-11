package jsonrpc

import (
	"context"
	"encoding/json"
)

type Handler interface {
	ServeJSONRPC(ctx context.Context, req Request, callback Requester) *Response
}

type HandleFunc func(ctx context.Context, req Request, callback Requester) *Response

func (h HandleFunc) ServeJSONRPC(ctx context.Context, req Request, callback Requester) *Response {
	return h(ctx, req, callback)
}

type Requester interface {
	AsyncRequestJSONRPC(ctx context.Context, method string, params interface{}) error
}

func HandleRequests(ctx context.Context, h Handler, callback Requester, reqs []Request, isBatch bool) ([]byte, error) {
	resps := make([]Response, 0, len(reqs))
	for _, req := range reqs {
		resp := h.ServeJSONRPC(ctx, req, callback)
		if resp != nil {
			resps = append(resps, *resp)
		}
	}

	if isBatch {
		return json.Marshal(resps)
	}
	if len(resps) > 0 {
		return json.Marshal(&resps[0])
	}
	return nil, nil
}
