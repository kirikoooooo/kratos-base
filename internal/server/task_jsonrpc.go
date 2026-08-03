package server

import (
	"context"
	"encoding/json"
	"net/http"

	taskv1 "kratos-demo/api/task/v1"
)

type taskRPC interface {
	CreateTask(ctx context.Context, req *taskv1.CreateTaskRequest) (*taskv1.TaskReply, error)
	GetTask(ctx context.Context, req *taskv1.GetTaskRequest) (*taskv1.TaskReply, error)
}

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func newTaskJSONRPCHandler(tasks taskRPC) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var request jsonRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.JSONRPC != "2.0" {
			writeJSONRPCError(w, request.ID, -32600, "Invalid Request")
			return
		}
		switch request.Method {
		case "tasks.create":
			var params taskv1.CreateTaskRequest
			if err := json.Unmarshal(request.Params, &params); err != nil {
				writeJSONRPCError(w, request.ID, -32602, "Invalid params")
				return
			}
			result, err := tasks.CreateTask(r.Context(), &params)
			if err != nil {
				writeJSONRPCError(w, request.ID, -32000, err.Error())
				return
			}
			writeJSONRPCResult(w, request.ID, result)
		case "tasks.get":
			var params taskv1.GetTaskRequest
			if err := json.Unmarshal(request.Params, &params); err != nil {
				writeJSONRPCError(w, request.ID, -32602, "Invalid params")
				return
			}
			result, err := tasks.GetTask(r.Context(), &params)
			if err != nil {
				writeJSONRPCError(w, request.ID, -32000, err.Error())
				return
			}
			writeJSONRPCResult(w, request.ID, result)
		default:
			writeJSONRPCError(w, request.ID, -32601, "Method not found")
		}
	})
}

func writeJSONRPCResult(w http.ResponseWriter, id json.RawMessage, result any) {
	writeJSONRPC(w, jsonRPCResponse{JSONRPC: "2.0", ID: id, Result: result})
}

func writeJSONRPCError(w http.ResponseWriter, id json.RawMessage, code int, message string) {
	writeJSONRPC(w, jsonRPCResponse{JSONRPC: "2.0", ID: id, Error: &jsonRPCError{Code: code, Message: message}})
}

func writeJSONRPC(w http.ResponseWriter, response jsonRPCResponse) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}
