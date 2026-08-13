package service

import (
	"context"
	"encoding/json"
	"net/http"

	taskv1 "kratos-demo/api/task/v1"
	"kratos-demo/app/dashboard/internal/biz"
)

// TaskService is the Dashboard-facing adapter around the runtime gRPC client.
type TaskService struct{ tasks biz.TaskClient }

func NewTaskService(tasks biz.TaskClient) *TaskService { return &TaskService{tasks: tasks} }

func (s *TaskService) CreateTask(ctx context.Context, request *taskv1.CreateTaskRequest) (*taskv1.TaskReply, error) {
	return s.tasks.CreateTask(ctx, request)
}

func (s *TaskService) GetTask(ctx context.Context, request *taskv1.GetTaskRequest) (*taskv1.TaskReply, error) {
	return s.tasks.GetTask(ctx, request)
}

// RegisterHTTP exposes a thin Dashboard facade. It never accesses runtime
// implementation objects; both handlers forward through TaskClient.
func (s *TaskService) RegisterHTTP(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/tasks", s.handleCreateTask)
	mux.HandleFunc("GET /api/v1/tasks/{taskID}", s.handleGetTask)
}

func (s *TaskService) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	request := new(taskv1.CreateTaskRequest)
	if err := json.NewDecoder(r.Body).Decode(request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	reply, err := s.CreateTask(r.Context(), request)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, reply)
}

func (s *TaskService) handleGetTask(w http.ResponseWriter, r *http.Request) {
	reply, err := s.GetTask(r.Context(), &taskv1.GetTaskRequest{TaskID: r.PathValue("taskID")})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, reply)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
