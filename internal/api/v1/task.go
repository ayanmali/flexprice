package v1

import (
	"net/http"

	"github.com/flexprice/flexprice/internal/api/dto"
	ierr "github.com/flexprice/flexprice/internal/errors"
	"github.com/flexprice/flexprice/internal/logger"
	"github.com/flexprice/flexprice/internal/service"
	"github.com/flexprice/flexprice/internal/temporal"
	"github.com/flexprice/flexprice/internal/types"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

type TaskHandler struct {
	service         service.TaskService
	temporalService *temporal.Service
	log             *logger.Logger
}

func NewTaskHandler(
	service service.TaskService,
	temporalService *temporal.Service,
	log *logger.Logger,
) *TaskHandler {
	return &TaskHandler{
		service:         service,
		temporalService: temporalService,
		log:             log,
	}
}

// @Summary Create a new task
// @Description Create a new task for processing files asynchronously
// @Tags Tasks
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param task body dto.CreateTaskRequest true "Task configuration"
// @Success 202 {object} dto.TaskResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /tasks [post]
func (h *TaskHandler) CreateTask(c *gin.Context) {
	var req dto.CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ierr.WithError(err).
			WithHint("Invalid request format").
			Mark(ierr.ErrValidation))
		return
	}

	resp, err := h.service.CreateTask(c.Request.Context(), req)
	if err != nil {
		c.Error(err)
		return
	}

	// Start the temporal workflow for async processing
	_, err = h.temporalService.StartTaskProcessingWorkflow(c.Request.Context(), resp.ID)
	if err != nil {
		h.log.Error("failed to start temporal workflow", "error", err, "task_id", resp.ID)
		// Don't fail the request, just log the error
		// The task will remain in PENDING status and can be retried
	}

	// Return 202 Accepted for async processing
	c.JSON(http.StatusAccepted, resp)
}

// @Summary Get a task
// @Description Get a task by ID
// @Tags Tasks
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Task ID"
// @Success 200 {object} dto.TaskResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /tasks/{id} [get]
func (h *TaskHandler) GetTask(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.Error(ierr.NewError("task ID is required").
			WithHint("Task ID is required").
			Mark(ierr.ErrValidation))
		return
	}

	resp, err := h.service.GetTask(c.Request.Context(), id)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// @Summary List tasks
// @Description List tasks with optional filtering
// @Tags Tasks
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param filter query types.TaskFilter false "Filter"
// @Success 200 {object} dto.ListTasksResponse
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /tasks [get]
func (h *TaskHandler) ListTasks(c *gin.Context) {
	var filter types.TaskFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.Error(ierr.WithError(err).
			WithHint("Invalid filter parameters").
			Mark(ierr.ErrValidation))
		return
	}

	if filter.GetLimit() == 0 {
		filter.Limit = lo.ToPtr(types.GetDefaultFilter().Limit)
	}

	resp, err := h.service.ListTasks(c.Request.Context(), &filter)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// @Summary Update task status
// @Description Update a task's status
// @Tags Tasks
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Task ID"
// @Param status body dto.UpdateTaskStatusRequest true "Status update"
// @Success 200 {object} gin.H
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /tasks/{id}/status [put]
func (h *TaskHandler) UpdateTaskStatus(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.Error(ierr.NewError("task ID is required").
			WithHint("Task ID is required").
			Mark(ierr.ErrValidation))
		return
	}

	var req dto.UpdateTaskStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ierr.WithError(err).
			WithHint("Invalid request format").
			Mark(ierr.ErrValidation))
		return
	}

	err := h.service.UpdateTaskStatus(c.Request.Context(), id, req.TaskStatus)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "task status updated successfully"})
}

// @Summary Process task with streaming
// @Description Process a task using streaming for large files
// @Tags Tasks
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "Task ID"
// @Success 200 {object} gin.H
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /tasks/{id}/process [post]
func (h *TaskHandler) ProcessTask(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.Error(ierr.NewError("task ID is required").
			WithHint("Task ID is required").
			Mark(ierr.ErrValidation))
		return
	}

	err := h.service.ProcessTaskWithStreaming(c.Request.Context(), id)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "task processing started with streaming"})
}

// @Summary Get task processing result
// @Description Get the result of a task processing workflow
// @Tags Tasks
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param workflow_id query string true "Workflow ID"
// @Success 200 {object} temporal.models.TaskProcessingWorkflowResult
// @Failure 400 {object} ierr.ErrorResponse
// @Failure 404 {object} ierr.ErrorResponse
// @Failure 500 {object} ierr.ErrorResponse
// @Router /tasks/result [get]
func (h *TaskHandler) GetTaskProcessingResult(c *gin.Context) {
	workflowID := c.Query("workflow_id")
	if workflowID == "" {
		c.Error(ierr.NewError("workflow_id is required").
			WithHint("Workflow ID is required").
			Mark(ierr.ErrValidation))
		return
	}

	result, err := h.temporalService.GetTaskProcessingWorkflowResult(c.Request.Context(), workflowID)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, result)
}
