package tasking

import (
	"strings"
	"time"

	taskv1 "kratos-demo/api/task/v1"
	"kratos-demo/internal/consts/public"
	"kratos-demo/internal/data/common"
)

func agentName(agent public.AgentKind) string {
	return string(agent)
}

func isEmptyAgent(agent public.AgentKind) bool {
	return strings.TrimSpace(agentName(agent)) == ""
}

func taskToCommand(task *Task) *taskv1.TaskCommand {
	if task == nil {
		return nil
	}
	return &taskv1.TaskCommand{
		TaskID: task.ID,
		Agent:  agentName(task.Agent),
		Prompt: task.Prompt,
	}
}

func markTaskRunning(task *Task, at time.Time) {
	if task == nil {
		return
	}
	task.Status = TaskStatusRunning
	task.Error = ""
	task.UpdatedAt = at
}

func markTaskFailed(task *Task, err error, at time.Time) {
	if task == nil {
		return
	}
	task.Status = TaskStatusFailed
	task.Result = nil
	task.UpdatedAt = at
	if err != nil {
		task.Error = err.Error()
		return
	}
	task.Error = ""
}

func markTaskDone(task *Task, result *taskv1.TaskResult, at time.Time) {
	if task == nil {
		return
	}
	task.Status = TaskStatusDone
	task.Result = result
	task.Error = ""
	task.UpdatedAt = at
}

func cloneTask(task *Task) *Task {
	if task == nil {
		return nil
	}
	copyTask := *task
	if task.Result != nil {
		result := *task.Result
		copyTask.Result = &result
	}
	return &copyTask
}

func previewPrompt(prompt string) string {
	return common.PreviewPrompt(prompt)
}
