package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func (h *SMQHandler) SetupRoutes(router *gin.Engine) {
	// Set Gin to release mode for production
	gin.SetMode(gin.ReleaseMode)

	// Setup middleware
	h.setupMiddleware(router)

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) { h.handleHealthCheck(c.Writer, c.Request) })

	// Authentication routes
	router.GET("/login", func(c *gin.Context) { h.handleLogin(c.Writer, c.Request) })
	router.POST("/login", func(c *gin.Context) { h.handleLogin(c.Writer, c.Request) })
	router.POST("/logout", func(c *gin.Context) { h.handleLogout(c.Writer, c.Request) })

	// Dashboard endpoint (protected)
	router.GET("/", func(c *gin.Context) { h.requireAuth(h.handleDashboard)(c.Writer, c.Request) })

	// Dashboard API routes group (protected)
	api := router.Group("/api")
	{
		api.GET("/status", func(c *gin.Context) { h.requireAuth(h.handleAPIStatus)(c.Writer, c.Request) })
		api.GET("/queues", func(c *gin.Context) { h.requireAuth(h.handleAPIListQueues)(c.Writer, c.Request) })
		api.POST("/queues", func(c *gin.Context) { h.requireAuth(h.handleAPICreateQueue)(c.Writer, c.Request) })
		api.DELETE("/queues/:name", func(c *gin.Context) { h.requireAuth(h.handleAPIDeleteQueue)(c.Writer, c.Request) })
		api.POST("/messages", func(c *gin.Context) { h.requireAuth(h.handleAPISendMessage)(c.Writer, c.Request) })
		api.POST("/messages/poll", func(c *gin.Context) { h.requireAuth(h.handleAPIPollMessages)(c.Writer, c.Request) })
		api.POST("/messages/send", func(c *gin.Context) { h.requireAuth(h.handleAPISendMessage)(c.Writer, c.Request) })
		api.DELETE("/messages/delete", func(c *gin.Context) { h.requireAuth(h.handleAPIDeleteMessage)(c.Writer, c.Request) })
		api.GET("/queues/:name/messages", func(c *gin.Context) { h.requireAuth(h.handleAPIGetQueueMessages)(c.Writer, c.Request) })

		// Access key management endpoints
		api.POST("/auth/access-keys", func(c *gin.Context) { h.requireAuth(h.handleCreateAccessKey)(c.Writer, c.Request) })
		api.GET("/auth/access-keys", func(c *gin.Context) { h.requireAuth(h.handleListAccessKeys)(c.Writer, c.Request) })
		api.POST("/auth/access-keys/deactivate", func(c *gin.Context) { h.requireAuth(h.handleDeactivateAccessKey)(c.Writer, c.Request) })
		api.DELETE("/auth/access-keys/:id", func(c *gin.Context) { h.requireAuth(h.handleDeleteAccessKey)(c.Writer, c.Request) })
	}

	// SQS protocol endpoint (form-encoded and JSON)
	router.POST("/", func(c *gin.Context) {
		h.authenticateRequest(func(w http.ResponseWriter, r *http.Request) {
			h.ginHandleSQSProtocol(c)
		})(c.Writer, c.Request)
	})
}

func (h *SMQHandler) setupMiddleware(router *gin.Engine) {
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"*"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	router.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		return fmt.Sprintf("DEBUG: %s %s\n", param.Method, param.Path)
	}))

	router.Use(gin.Recovery())
}

func (h *SMQHandler) ginHandleSQSProtocol(c *gin.Context) {
	contentType := c.GetHeader("Content-Type")
	isJSONProtocol := strings.Contains(contentType, "application/x-amz-json-1.0")

	if isJSONProtocol {
		h.handleJSONRequest(c.Writer, c.Request)
		return
	}

	if err := c.Request.ParseForm(); err != nil {
		h.writeErrorResponse(c.Writer, "InvalidRequest", "Failed to parse form data", http.StatusBadRequest)
		return
	}

	action := c.Request.FormValue("Action")

	switch action {
	case "CreateQueue":
		h.handleCreateQueue(c.Writer, c.Request)
	case "DeleteQueue":
		h.handleDeleteQueue(c.Writer, c.Request)
	case "ListQueues":
		h.handleListQueues(c.Writer, c.Request)
	case "GetQueueUrl":
		h.handleGetQueueUrl(c.Writer, c.Request)
	case "SendMessage":
		h.handleSendMessage(c.Writer, c.Request)
	case "SendMessageBatch":
		h.handleSendMessageBatch(c.Writer, c.Request)
	case "ReceiveMessage":
		h.handleReceiveMessage(c.Writer, c.Request)
	case "DeleteMessage":
		h.handleDeleteMessage(c.Writer, c.Request)
	case "DeleteMessageBatch":
		h.handleDeleteMessageBatch(c.Writer, c.Request)
	case "ChangeMessageVisibility":
		h.handleChangeMessageVisibility(c.Writer, c.Request)
	case "ChangeMessageVisibilityBatch":
		h.handleChangeMessageVisibilityBatch(c.Writer, c.Request)
	case "GetQueueAttributes":
		h.handleGetQueueAttributes(c.Writer, c.Request)
	case "SetQueueAttributes":
		h.handleSetQueueAttributes(c.Writer, c.Request)
	case "PurgeQueue":
		h.handlePurgeQueue(c.Writer, c.Request)
	default:
		h.writeErrorResponse(c.Writer, "InvalidAction", fmt.Sprintf("Unknown action: %s", action), http.StatusBadRequest)
	}
}

func (h *SMQHandler) handleJSONRequest(w http.ResponseWriter, r *http.Request) {
	// Extract action from x-amz-target header (case-insensitive)
	target := r.Header.Get("X-Amz-Target")
	if target == "" {
		target = r.Header.Get("x-amz-target")
	}
	if target == "" {
		h.writeJSONErrorResponse(w, "InvalidAction", "Missing x-amz-target header", http.StatusBadRequest)
		return
	}

	// Parse action from target (e.g., "AmazonSQS.SendMessage" -> "SendMessage")
	parts := strings.Split(target, ".")
	if len(parts) != 2 || parts[0] != "AmazonSQS" {
		h.writeJSONErrorResponse(w, "InvalidAction", fmt.Sprintf("Invalid target: %s", target), http.StatusBadRequest)
		return
	}
	action := parts[1]

	// Route to appropriate handler based on action
	switch action {
	case "CreateQueue":
		h.handleJSONCreateQueue(w, r)
	case "DeleteQueue":
		h.handleJSONDeleteQueue(w, r)
	case "ListQueues":
		h.handleJSONListQueues(w, r)
	case "GetQueueUrl":
		h.handleJSONGetQueueUrl(w, r)
	case "SendMessage":
		h.handleJSONSendMessage(w, r)
	case "SendMessageBatch":
		h.handleJSONSendMessageBatch(w, r)
	case "ReceiveMessage":
		h.handleJSONReceiveMessage(w, r)
	case "DeleteMessage":
		h.handleJSONDeleteMessage(w, r)
	case "DeleteMessageBatch":
		h.handleJSONDeleteMessageBatch(w, r)
	case "ChangeMessageVisibility":
		h.handleJSONChangeMessageVisibility(w, r)
	case "ChangeMessageVisibilityBatch":
		h.handleJSONChangeMessageVisibilityBatch(w, r)
	case "GetQueueAttributes":
		h.handleJSONGetQueueAttributes(w, r)
	case "SetQueueAttributes":
		h.handleJSONSetQueueAttributes(w, r)
	case "PurgeQueue":
		h.handleJSONPurgeQueue(w, r)
	default:
		h.writeJSONErrorResponse(w, "InvalidAction", fmt.Sprintf("Unsupported action: %s", action), http.StatusBadRequest)
	}
}
