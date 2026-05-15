package server

import (
	stderrors "errors"
	"net/http"

	"kratos-demo/internal/biz"
	"kratos-demo/internal/service"

	httptransport "github.com/go-kratos/kratos/v2/transport/http"
)

func registerTaskHTTPServer(s *httptransport.Server, srv *service.TaskService) {
	r := s.Route("/api/v1")
	r.POST("/tasks", createTaskHandler(srv))
	r.GET("/tasks/{task_id}", getTaskHandler(srv))
}

func createTaskHandler(srv *service.TaskService) func(ctx httptransport.Context) error {
	return func(ctx httptransport.Context) error {
		var req service.CreateTaskRequest
		if err := ctx.Bind(&req); err != nil {
			return ctx.JSON(http.StatusBadRequest, map[string]any{
				"error": err.Error(),
			})
		}

		reply, err := srv.CreateTask(ctx, &req)
		if err != nil {
			return writeTaskError(ctx, err)
		}
		return ctx.JSON(http.StatusAccepted, reply)
	}
}

func getTaskHandler(srv *service.TaskService) func(ctx httptransport.Context) error {
	return func(ctx httptransport.Context) error {
		taskID := ctx.Vars().Get("task_id")
		reply, err := srv.GetTask(ctx, taskID)
		if err != nil {
			return writeTaskError(ctx, err)
		}
		return ctx.JSON(http.StatusOK, reply)
	}
}

func writeTaskError(ctx httptransport.Context, err error) error {
	switch {
	case stderrors.Is(err, biz.ErrPromptRequired), stderrors.Is(err, biz.ErrAgentNotSupported):
		return ctx.JSON(http.StatusBadRequest, map[string]any{
			"error": err.Error(),
		})
	case stderrors.Is(err, biz.ErrTaskNotFound):
		return ctx.JSON(http.StatusNotFound, map[string]any{
			"error": err.Error(),
		})
	default:
		return ctx.JSON(http.StatusInternalServerError, map[string]any{
			"error": err.Error(),
		})
	}
}
