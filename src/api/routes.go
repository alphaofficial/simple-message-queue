package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// SetupRoutes configures all routes for the Gin router
func (h *SQSHandler) SetupRoutes(router *gin.Engine) {
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
	router.POST("/", h.ginHandleSQSProtocol)
}

// setupMiddleware configures Gin middleware
func (h *SQSHandler) setupMiddleware(router *gin.Engine) {
	// CORS middleware
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"*"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	// Custom request logging
	router.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		return fmt.Sprintf("DEBUG: %s %s\n", param.Method, param.Path)
	}))

	// Recovery middleware
	router.Use(gin.Recovery())
}

// ginHandleSQSProtocol handles all SQS protocol requests (form-encoded and JSON)
func (h *SQSHandler) ginHandleSQSProtocol(c *gin.Context) {
	// Detect protocol based on Content-Type
	contentType := c.GetHeader("Content-Type")
	isJSONProtocol := strings.Contains(contentType, "application/x-amz-json-1.0")

	if isJSONProtocol {
		h.handleJSONRequest(c.Writer, c.Request)
		return
	}

	// Handle traditional form-encoded SQS requests
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

// Legacy method for backwards compatibility - will be removed
func (h *SQSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("WARNING: ServeHTTP called - this should not happen with Gin router\n")
	fmt.Printf("DEBUG: %s %s\n", r.Method, r.URL.Path)

	// Handle health check
	if r.Method == http.MethodGet && r.URL.Path == "/health" {
		h.handleHealthCheck(w, r)
		return
	}

	// Handle dashboard requests
	if r.Method == http.MethodGet && r.URL.Path == "/" {
		h.handleDashboard(w, r)
		return
	}

	// API endpoints for dashboard
	if strings.HasPrefix(r.URL.Path, "/api/") {
		fmt.Printf("DEBUG: Routing to API handler\n")
		h.handleAPIRoutes(w, r)
		return
	}

	// SQS API endpoints (both JSON and XML protocols)
	if r.Method != http.MethodPost {
		h.writeErrorResponse(w, "InvalidAction", "Only POST method is supported", http.StatusMethodNotAllowed)
		return
	}

	// Detect protocol based on Content-Type
	contentType := r.Header.Get("Content-Type")
	isJSONProtocol := strings.Contains(contentType, "application/x-amz-json-1.0")

	if isJSONProtocol {
		h.handleJSONRequest(w, r)
		return
	}

	// Handle traditional form-encoded SQS requests
	if err := r.ParseForm(); err != nil {
		h.writeErrorResponse(w, "InvalidRequest", "Failed to parse form data", http.StatusBadRequest)
		return
	}

	action := r.FormValue("Action")

	switch action {
	case "CreateQueue":
		h.handleCreateQueue(w, r)
	case "DeleteQueue":
		h.handleDeleteQueue(w, r)
	case "ListQueues":
		h.handleListQueues(w, r)
	case "GetQueueUrl":
		h.handleGetQueueUrl(w, r)
	case "SendMessage":
		h.handleSendMessage(w, r)
	case "SendMessageBatch":
		h.handleSendMessageBatch(w, r)
	case "ReceiveMessage":
		h.handleReceiveMessage(w, r)
	case "DeleteMessage":
		h.handleDeleteMessage(w, r)
	case "DeleteMessageBatch":
		h.handleDeleteMessageBatch(w, r)
	case "ChangeMessageVisibility":
		h.handleChangeMessageVisibility(w, r)
	case "ChangeMessageVisibilityBatch":
		h.handleChangeMessageVisibilityBatch(w, r)
	case "GetQueueAttributes":
		h.handleGetQueueAttributes(w, r)
	case "SetQueueAttributes":
		h.handleSetQueueAttributes(w, r)
	case "PurgeQueue":
		h.handlePurgeQueue(w, r)
	default:
		h.writeErrorResponse(w, "InvalidAction", fmt.Sprintf("Unknown action: %s", action), http.StatusBadRequest)
	}
}

// handleJSONRequest routes JSON protocol requests to appropriate handlers
func (h *SQSHandler) handleJSONRequest(w http.ResponseWriter, r *http.Request) {
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

// handleAPIRoutes handles dashboard API routes
func (h *SQSHandler) handleAPIRoutes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch r.URL.Path {
	case "/api/status":
		if r.Method == http.MethodGet {
			h.handleAPIStatus(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	case "/api/queues":
		if r.Method == http.MethodGet {
			h.handleAPIListQueues(w, r)
		} else if r.Method == http.MethodPost {
			h.handleAPICreateQueue(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	case "/api/messages":
		if r.Method == http.MethodPost {
			h.handleAPISendMessage(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	case "/api/messages/poll":
		if r.Method == http.MethodPost {
			h.handleAPIPollMessages(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	case "/api/messages/send":
		if r.Method == http.MethodPost {
			h.handleAPISendMessage(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	default:
		if strings.HasPrefix(r.URL.Path, "/api/queues/") {
			if strings.HasSuffix(r.URL.Path, "/messages") {
				if r.Method == http.MethodGet {
					h.handleAPIGetQueueMessages(w, r)
				} else {
					http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				}
			} else {
				// Handle individual queue operations like DELETE /api/queues/{name}
				if r.Method == http.MethodDelete {
					h.handleAPIDeleteQueue(w, r)
				} else {
					http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				}
			}
		} else {
			http.Error(w, "Not found", http.StatusNotFound)
		}
	}
}
