package api

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	storage "github.com/sqs-producer/sqs-server/internal/storage/interface"
)

type SQSHandler struct {
	storage storage.Storage
	baseURL string
}

func NewSQSHandler(storage storage.Storage, baseURL string) *SQSHandler {
	return &SQSHandler{
		storage: storage,
		baseURL: baseURL,
	}
}

func (h *SQSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Handle dashboard requests
	if r.Method == http.MethodGet && r.URL.Path == "/" {
		h.handleDashboard(w, r)
		return
	}

	if r.Method == http.MethodGet && r.URL.Path == "/api/status" {
		h.handleAPIStatus(w, r)
		return
	}

	if r.Method != http.MethodPost {
		h.writeErrorResponse(w, "InvalidAction", "Only POST method is supported", http.StatusMethodNotAllowed)
		return
	}

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
	case "ReceiveMessage":
		h.handleReceiveMessage(w, r)
	case "DeleteMessage":
		h.handleDeleteMessage(w, r)
	case "ChangeMessageVisibility":
		h.handleChangeMessageVisibility(w, r)
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

func (h *SQSHandler) handleCreateQueue(w http.ResponseWriter, r *http.Request) {
	queueName := r.FormValue("QueueName")
	if queueName == "" {
		h.writeErrorResponse(w, "MissingParameter", "QueueName is required", http.StatusBadRequest)
		return
	}

	// Parse attributes
	attributes := make(map[string]string)
	for key, values := range r.Form {
		if strings.HasPrefix(key, "Attribute.") && strings.HasSuffix(key, ".Name") {
			attrIndex := strings.TrimPrefix(key, "Attribute.")
			attrIndex = strings.TrimSuffix(attrIndex, ".Name")

			attrName := values[0]
			attrValueKey := fmt.Sprintf("Attribute.%s.Value", attrIndex)
			if attrValues, ok := r.Form[attrValueKey]; ok && len(attrValues) > 0 {
				attributes[attrName] = attrValues[0]
			}
		}
	}

	// Create queue object
	queue := &storage.Queue{
		Name:                          queueName,
		URL:                           fmt.Sprintf("%s/%s", h.baseURL, queueName),
		Attributes:                    attributes,
		VisibilityTimeoutSeconds:      30,
		MessageRetentionPeriod:        1209600, // 14 days
		MaxReceiveCount:               0,
		DelaySeconds:                  0,
		ReceiveMessageWaitTimeSeconds: 0,
		CreatedAt:                     time.Now(),
	}

	// Parse standard attributes
	if val, ok := attributes["VisibilityTimeout"]; ok {
		if timeout, err := strconv.Atoi(val); err == nil {
			queue.VisibilityTimeoutSeconds = timeout
		}
	}
	if val, ok := attributes["MaxReceiveCount"]; ok {
		if count, err := strconv.Atoi(val); err == nil {
			queue.MaxReceiveCount = count
		}
	}
	if val, ok := attributes["RedrivePolicy"]; ok {
		queue.RedrivePolicy = val
	}

	if err := h.storage.CreateQueue(r.Context(), queue); err != nil {
		h.writeErrorResponse(w, "QueueAlreadyExists", "Queue already exists", http.StatusBadRequest)
		return
	}

	response := CreateQueueResponse{
		QueueURL: queue.URL,
		SQSResponse: SQSResponse{
			RequestId: uuid.New().String(),
		},
	}

	h.writeXMLResponse(w, response, http.StatusOK)
}

func (h *SQSHandler) handleDeleteQueue(w http.ResponseWriter, r *http.Request) {
	queueURL := r.FormValue("QueueUrl")
	queueName := h.extractQueueNameFromURL(queueURL)

	if err := h.storage.DeleteQueue(r.Context(), queueName); err != nil {
		h.writeErrorResponse(w, "QueueDoesNotExist", "Queue does not exist", http.StatusBadRequest)
		return
	}

	response := SQSResponse{
		RequestId: uuid.New().String(),
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	xml.NewEncoder(w).Encode(response)
}

func (h *SQSHandler) handleListQueues(w http.ResponseWriter, r *http.Request) {
	prefix := r.FormValue("QueueNamePrefix")

	queues, err := h.storage.ListQueues(r.Context(), prefix)
	if err != nil {
		h.writeErrorResponse(w, "InternalError", "Failed to list queues", http.StatusInternalServerError)
		return
	}

	var queueURLs []string
	for _, queue := range queues {
		queueURLs = append(queueURLs, queue.URL)
	}

	response := ListQueuesResponse{
		QueueURLs: queueURLs,
		SQSResponse: SQSResponse{
			RequestId: uuid.New().String(),
		},
	}

	h.writeXMLResponse(w, response, http.StatusOK)
}

func (h *SQSHandler) handleGetQueueUrl(w http.ResponseWriter, r *http.Request) {
	queueName := r.FormValue("QueueName")

	queue, err := h.storage.GetQueue(r.Context(), queueName)
	if err != nil || queue == nil {
		h.writeErrorResponse(w, "QueueDoesNotExist", "Queue does not exist", http.StatusBadRequest)
		return
	}

	response := CreateQueueResponse{
		QueueURL: queue.URL,
		SQSResponse: SQSResponse{
			RequestId: uuid.New().String(),
		},
	}

	h.writeXMLResponse(w, response, http.StatusOK)
}

func (h *SQSHandler) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	queueURL := r.FormValue("QueueUrl")
	queueName := h.extractQueueNameFromURL(queueURL)
	messageBody := r.FormValue("MessageBody")

	if messageBody == "" {
		h.writeErrorResponse(w, "MissingParameter", "MessageBody is required", http.StatusBadRequest)
		return
	}

	// Get queue to check max receive count for DLQ
	queue, err := h.storage.GetQueue(r.Context(), queueName)
	if err != nil || queue == nil {
		h.writeErrorResponse(w, "QueueDoesNotExist", "Queue does not exist", http.StatusBadRequest)
		return
	}

	// Create message
	message := &storage.Message{
		ID:                uuid.New().String(),
		QueueName:         queueName,
		Body:              messageBody,
		Attributes:        make(map[string]string),
		MessageAttributes: make(map[string]storage.MessageAttribute),
		ReceiptHandle:     uuid.New().String(),
		ReceiveCount:      0,
		MaxReceiveCount:   queue.MaxReceiveCount,
		CreatedAt:         time.Now(),
		DelaySeconds:      0,
	}

	// Calculate MD5
	hash := md5.Sum([]byte(messageBody))
	message.MD5OfBody = hex.EncodeToString(hash[:])

	// Parse delay seconds
	if delayStr := r.FormValue("DelaySeconds"); delayStr != "" {
		if delay, err := strconv.Atoi(delayStr); err == nil {
			message.DelaySeconds = delay
		}
	}

	// Parse message attributes
	for key, values := range r.Form {
		if strings.HasPrefix(key, "MessageAttribute.") && strings.HasSuffix(key, ".Name") {
			attrIndex := strings.TrimPrefix(key, "MessageAttribute.")
			attrIndex = strings.TrimSuffix(attrIndex, ".Name")

			attrName := values[0]
			dataTypeKey := fmt.Sprintf("MessageAttribute.%s.Value.DataType", attrIndex)
			stringValueKey := fmt.Sprintf("MessageAttribute.%s.Value.StringValue", attrIndex)

			attr := storage.MessageAttribute{}
			if dataType, ok := r.Form[dataTypeKey]; ok && len(dataType) > 0 {
				attr.DataType = dataType[0]
			}
			if stringValue, ok := r.Form[stringValueKey]; ok && len(stringValue) > 0 {
				attr.StringValue = stringValue[0]
			}

			message.MessageAttributes[attrName] = attr
		}
	}

	if err := h.storage.SendMessage(r.Context(), message); err != nil {
		h.writeErrorResponse(w, "InternalError", "Failed to send message", http.StatusInternalServerError)
		return
	}

	response := SendMessageResponse{
		MessageId:              message.ID,
		MD5OfBody:              message.MD5OfBody,
		MD5OfMessageAttributes: message.MD5OfAttributes,
		SQSResponse: SQSResponse{
			RequestId: uuid.New().String(),
		},
	}

	h.writeXMLResponse(w, response, http.StatusOK)
}

func (h *SQSHandler) handleReceiveMessage(w http.ResponseWriter, r *http.Request) {
	queueURL := r.FormValue("QueueUrl")
	queueName := h.extractQueueNameFromURL(queueURL)

	maxMessages := 1
	if maxStr := r.FormValue("MaxNumberOfMessages"); maxStr != "" {
		if max, err := strconv.Atoi(maxStr); err == nil && max > 0 && max <= 10 {
			maxMessages = max
		}
	}

	waitTimeSeconds := 0
	if waitStr := r.FormValue("WaitTimeSeconds"); waitStr != "" {
		if wait, err := strconv.Atoi(waitStr); err == nil {
			waitTimeSeconds = wait
		}
	}

	messages, err := h.storage.ReceiveMessages(r.Context(), queueName, maxMessages, waitTimeSeconds)
	if err != nil {
		h.writeErrorResponse(w, "InternalError", "Failed to receive messages", http.StatusInternalServerError)
		return
	}

	var responseMessages []Message
	for _, msg := range messages {
		responseMessages = append(responseMessages, Message{
			MessageId:         msg.ID,
			ReceiptHandle:     msg.ReceiptHandle,
			MD5OfBody:         msg.MD5OfBody,
			Body:              msg.Body,
			Attributes:        msg.Attributes,
			MessageAttributes: convertMessageAttributes(msg.MessageAttributes),
		})
	}

	response := ReceiveMessageResponse{
		Messages: responseMessages,
		SQSResponse: SQSResponse{
			RequestId: uuid.New().String(),
		},
	}

	h.writeXMLResponse(w, response, http.StatusOK)
}

func (h *SQSHandler) handleDeleteMessage(w http.ResponseWriter, r *http.Request) {
	queueURL := r.FormValue("QueueUrl")
	queueName := h.extractQueueNameFromURL(queueURL)
	receiptHandle := r.FormValue("ReceiptHandle")

	if receiptHandle == "" {
		h.writeErrorResponse(w, "MissingParameter", "ReceiptHandle is required", http.StatusBadRequest)
		return
	}

	if err := h.storage.DeleteMessage(r.Context(), queueName, receiptHandle); err != nil {
		h.writeErrorResponse(w, "InternalError", "Failed to delete message", http.StatusInternalServerError)
		return
	}

	response := DeleteMessageResponse{
		SQSResponse: SQSResponse{
			RequestId: uuid.New().String(),
		},
	}

	h.writeXMLResponse(w, response, http.StatusOK)
}

func (h *SQSHandler) handleChangeMessageVisibility(w http.ResponseWriter, r *http.Request) {
	queueURL := r.FormValue("QueueUrl")
	queueName := h.extractQueueNameFromURL(queueURL)
	receiptHandle := r.FormValue("ReceiptHandle")
	visibilityTimeoutStr := r.FormValue("VisibilityTimeout")

	visibilityTimeout, err := strconv.Atoi(visibilityTimeoutStr)
	if err != nil {
		h.writeErrorResponse(w, "InvalidParameterValue", "Invalid VisibilityTimeout", http.StatusBadRequest)
		return
	}

	if err := h.storage.ChangeMessageVisibility(r.Context(), queueName, receiptHandle, visibilityTimeout); err != nil {
		h.writeErrorResponse(w, "InternalError", "Failed to change message visibility", http.StatusInternalServerError)
		return
	}

	response := SQSResponse{
		RequestId: uuid.New().String(),
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	xml.NewEncoder(w).Encode(response)
}

func (h *SQSHandler) handleGetQueueAttributes(w http.ResponseWriter, r *http.Request) {
	// Simplified implementation - would need to be expanded
	response := SQSResponse{
		RequestId: uuid.New().String(),
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	xml.NewEncoder(w).Encode(response)
}

func (h *SQSHandler) handleSetQueueAttributes(w http.ResponseWriter, r *http.Request) {
	// Simplified implementation - would need to be expanded
	response := SQSResponse{
		RequestId: uuid.New().String(),
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	xml.NewEncoder(w).Encode(response)
}

func (h *SQSHandler) handlePurgeQueue(w http.ResponseWriter, r *http.Request) {
	queueURL := r.FormValue("QueueUrl")
	queueName := h.extractQueueNameFromURL(queueURL)

	if err := h.storage.PurgeQueue(r.Context(), queueName); err != nil {
		h.writeErrorResponse(w, "InternalError", "Failed to purge queue", http.StatusInternalServerError)
		return
	}

	response := SQSResponse{
		RequestId: uuid.New().String(),
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	xml.NewEncoder(w).Encode(response)
}

func (h *SQSHandler) extractQueueNameFromURL(queueURL string) string {
	if queueURL == "" {
		return ""
	}

	u, err := url.Parse(queueURL)
	if err != nil {
		return ""
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}

	return ""
}

func (h *SQSHandler) writeErrorResponse(w http.ResponseWriter, code, message string, statusCode int) {
	errorResp := ErrorResponse{
		Error: Error{
			Type:    "Sender",
			Code:    code,
			Message: message,
		},
		RequestId: uuid.New().String(),
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(statusCode)
	xml.NewEncoder(w).Encode(errorResp)
}

func (h *SQSHandler) writeXMLResponse(w http.ResponseWriter, response interface{}, statusCode int) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(statusCode)

	w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>`))
	xml.NewEncoder(w).Encode(response)
}

func convertMessageAttributes(attrs map[string]storage.MessageAttribute) map[string]MessageAttribute {
	result := make(map[string]MessageAttribute)
	for k, v := range attrs {
		result[k] = MessageAttribute{
			DataType:    v.DataType,
			StringValue: v.StringValue,
			BinaryValue: v.BinaryValue,
		}
	}
	return result
}

func (h *SQSHandler) handleDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)

	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>SQS Server Dashboard</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: #f5f5f5;
            color: #333;
            line-height: 1.6;
        }
        
        .header {
            background: #2c3e50;
            color: white;
            padding: 1rem 0;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        
        .container {
            max-width: 1200px;
            margin: 0 auto;
            padding: 0 1rem;
        }
        
        .header h1 {
            font-size: 1.8rem;
            font-weight: 300;
        }
        
        .main {
            padding: 2rem 0;
        }
        
        .dashboard-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
            gap: 1.5rem;
            margin-bottom: 2rem;
        }
        
        .card {
            background: white;
            border-radius: 8px;
            box-shadow: 0 2px 8px rgba(0,0,0,0.1);
            padding: 1.5rem;
        }
        
        .card h2 {
            color: #2c3e50;
            margin-bottom: 1rem;
            font-size: 1.2rem;
            font-weight: 500;
        }
        
        .stat-item {
            display: flex;
            justify-content: space-between;
            padding: 0.5rem 0;
            border-bottom: 1px solid #eee;
        }
        
        .stat-item:last-child {
            border-bottom: none;
        }
        
        .stat-value {
            font-weight: 600;
            color: #27ae60;
        }
        
        .queue-list {
            list-style: none;
        }
        
        .queue-item {
            padding: 0.75rem;
            margin: 0.5rem 0;
            background: #f8f9fa;
            border-radius: 4px;
            border-left: 4px solid #3498db;
        }
        
        .queue-name {
            font-weight: 600;
            color: #2c3e50;
        }
        
        .queue-stats {
            font-size: 0.9rem;
            color: #666;
            margin-top: 0.25rem;
        }
        
        .messages-table {
            width: 100%;
            border-collapse: collapse;
            margin-top: 1rem;
        }
        
        .messages-table th,
        .messages-table td {
            padding: 0.75rem;
            text-align: left;
            border-bottom: 1px solid #eee;
        }
        
        .messages-table th {
            background: #f8f9fa;
            font-weight: 600;
            color: #2c3e50;
        }
        
        .message-body {
            max-width: 300px;
            overflow: hidden;
            text-overflow: ellipsis;
            white-space: nowrap;
        }
        
        .status-badge {
            padding: 0.25rem 0.5rem;
            border-radius: 12px;
            font-size: 0.8rem;
            font-weight: 500;
        }
        
        .status-available {
            background: #d4edda;
            color: #155724;
        }
        
        .status-inflight {
            background: #fff3cd;
            color: #856404;
        }
        
        .refresh-btn {
            background: #3498db;
            color: white;
            border: none;
            padding: 0.5rem 1rem;
            border-radius: 4px;
            cursor: pointer;
            font-size: 0.9rem;
        }
        
        .refresh-btn:hover {
            background: #2980b9;
        }
        
        .auto-refresh {
            margin-left: 1rem;
            font-size: 0.9rem;
        }
        
        .controls {
            display: flex;
            align-items: center;
            margin-bottom: 1rem;
        }
        
        .queue-messages-section {
            margin-bottom: 1.5rem;
            padding-bottom: 1rem;
            border-bottom: 1px solid #eee;
        }
        
        .queue-messages-section:last-child {
            border-bottom: none;
        }
    </style>
</head>
<body>
    <div class="header">
        <div class="container">
            <h1>🚀 SQS Server Dashboard</h1>
        </div>
    </div>
    
    <div class="main">
        <div class="container">
            <div class="controls">
                <button class="refresh-btn" onclick="refreshData()">Refresh</button>
                <div class="auto-refresh">
                    <input type="checkbox" id="autoRefresh" onchange="toggleAutoRefresh()">
                    <label for="autoRefresh">Auto-refresh (5s)</label>
                </div>
            </div>
            
            <div class="dashboard-grid">
                <div class="card">
                    <h2>📊 Server Statistics</h2>
                    <div id="serverStats">
                        <div class="stat-item">
                            <span>Server Status</span>
                            <span class="stat-value">Running</span>
                        </div>
                        <div class="stat-item">
                            <span>Uptime</span>
                            <span class="stat-value" id="uptime">-</span>
                        </div>
                        <div class="stat-item">
                            <span>Total Queues</span>
                            <span class="stat-value" id="totalQueues">-</span>
                        </div>
                        <div class="stat-item">
                            <span>Total Messages</span>
                            <span class="stat-value" id="totalMessages">-</span>
                        </div>
                        <div class="stat-item">
                            <span>Messages in Flight</span>
                            <span class="stat-value" id="messagesInFlight">-</span>
                        </div>
                    </div>
                </div>
                
                <div class="card">
                    <h2>📋 Queue Overview</h2>
                    <ul class="queue-list" id="queueList">
                        <li>Loading queues...</li>
                    </ul>
                </div>
            </div>
            
            <div class="card">
                <h2>💬 Messages in Flight by Queue</h2>
                <div id="messagesByQueue">
                    <div style="text-align: center; padding: 2rem;">Loading messages...</div>
                </div>
            </div>
        </div>
    </div>
    
    <script>
        let autoRefreshInterval;
        const startTime = Date.now();
        
        function formatUptime(startTime) {
            const uptime = Math.floor((Date.now() - startTime) / 1000);
            const hours = Math.floor(uptime / 3600);
            const minutes = Math.floor((uptime % 3600) / 60);
            const seconds = uptime % 60;
            return hours + 'h ' + minutes + 'm ' + seconds + 's';
        }
        
        function updateUptime() {
            document.getElementById('uptime').textContent = formatUptime(startTime);
        }
        
        async function fetchStatus() {
            try {
                const response = await fetch('/api/status');
                const data = await response.json();
                
                // Update server stats
                document.getElementById('totalQueues').textContent = data.totalQueues || 0;
                document.getElementById('totalMessages').textContent = data.totalMessages || 0;
                document.getElementById('messagesInFlight').textContent = data.messagesInFlight || 0;
                
                // Update queue list
                const queueList = document.getElementById('queueList');
                if (data.queues && data.queues.length > 0) {
                    queueList.innerHTML = data.queues.map(queue => 
                        '<li class="queue-item">' +
                            '<div class="queue-name">' + queue.name + '</div>' +
                            '<div class="queue-stats">' +
                                'Available: ' + (queue.availableMessages || 0) + ' | ' +
                                'In Flight: ' + (queue.inFlightMessages || 0) + 
                            '</div>' +
                        '</li>'
                    ).join('');
                } else {
                    queueList.innerHTML = '<li>No queues found</li>';
                }
                
                // Update messages by queue
                const messagesByQueue = document.getElementById('messagesByQueue');
                if (data.queues && data.queues.length > 0) {
                    let html = '';
                    data.queues.forEach(queue => {
                        html += '<div class="queue-messages-section">';
                        html += '<h3 style="color: #2c3e50; margin: 1rem 0 0.5rem 0; font-size: 1.1rem;">' + queue.name + '</h3>';
                        
                        if (queue.messages && queue.messages.length > 0) {
                            html += '<table class="messages-table">';
                            html += '<thead><tr>';
                            html += '<th>Message ID</th>';
                            html += '<th>Body Preview</th>';
                            html += '<th>Receive Count</th>';
                            html += '<th>Created At</th>';
                            html += '</tr></thead><tbody>';
                            
                            queue.messages.forEach(msg => {
                                html += '<tr>';
                                html += '<td>' + msg.id.substring(0, 8) + '...</td>';
                                html += '<td class="message-body" title="' + msg.body + '">' + msg.body + '</td>';
                                html += '<td>' + msg.receiveCount + '</td>';
                                html += '<td>' + new Date(msg.createdAt).toLocaleString() + '</td>';
                                html += '</tr>';
                            });
                            
                            html += '</tbody></table>';
                        } else {
                            html += '<p style="color: #666; font-style: italic; margin: 0.5rem 0;">No messages in flight</p>';
                        }
                        html += '</div>';
                    });
                    messagesByQueue.innerHTML = html;
                } else {
                    messagesByQueue.innerHTML = '<div style="text-align: center; padding: 2rem;">No queues found</div>';
                }
                
            } catch (error) {
                console.error('Failed to fetch status:', error);
            }
        }
        
        function refreshData() {
            fetchStatus();
            updateUptime();
        }
        
        function toggleAutoRefresh() {
            const checkbox = document.getElementById('autoRefresh');
            if (checkbox.checked) {
                autoRefreshInterval = setInterval(refreshData, 5000);
            } else {
                clearInterval(autoRefreshInterval);
            }
        }
        
        // Initial load
        refreshData();
        
        // Update uptime every second
        setInterval(updateUptime, 1000);
    </script>
</body>
</html>`

	w.Write([]byte(html))
}

func (h *SQSHandler) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	// Get all queues
	queues, err := h.storage.ListQueues(r.Context(), "")
	if err != nil {
		http.Error(w, "Failed to fetch queues", http.StatusInternalServerError)
		return
	}

	totalMessages := 0
	totalMessagesInFlight := 0
	var queueStats []map[string]interface{}

	// Get statistics for each queue
	for _, queue := range queues {
		// Get messages for this queue
		messages, err := h.storage.GetInFlightMessages(r.Context(), queue.Name)
		if err != nil {
			continue
		}

		availableCount := 0
		inFlightCount := 0
		var queueInFlightMessages []map[string]interface{}

		for _, msg := range messages {
			totalMessages++
			if msg.VisibleAt != nil && msg.VisibleAt.After(time.Now()) {
				inFlightCount++
				totalMessagesInFlight++

				// Add to queue's in-flight messages list
				queueInFlightMessages = append(queueInFlightMessages, map[string]interface{}{
					"id":           msg.ID,
					"body":         msg.Body,
					"receiveCount": msg.ReceiveCount,
					"createdAt":    msg.CreatedAt,
				})
			} else {
				availableCount++
			}
		}

		queueStats = append(queueStats, map[string]interface{}{
			"name":              queue.Name,
			"availableMessages": availableCount,
			"inFlightMessages":  inFlightCount,
			"totalMessages":     availableCount + inFlightCount,
			"messages":          queueInFlightMessages,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	// Simple JSON encoding without importing encoding/json
	jsonStr := fmt.Sprintf(`{
		"totalQueues": %d,
		"totalMessages": %d,
		"messagesInFlight": %d,
		"queues": [`, len(queues), totalMessages, totalMessagesInFlight)

	for i, queue := range queueStats {
		if i > 0 {
			jsonStr += ","
		}
		jsonStr += fmt.Sprintf(`{
			"name": "%s",
			"availableMessages": %v,
			"inFlightMessages": %v,
			"totalMessages": %v,
			"messages": [`, queue["name"], queue["availableMessages"], queue["inFlightMessages"], queue["totalMessages"])

		// Add messages for this queue
		if msgs, ok := queue["messages"].([]map[string]interface{}); ok {
			for j, msg := range msgs {
				if j > 0 {
					jsonStr += ","
				}
				jsonStr += fmt.Sprintf(`{
					"id": "%s",
					"body": "%s",
					"receiveCount": %v,
					"createdAt": "%s"
				}`, msg["id"], strings.ReplaceAll(fmt.Sprintf("%v", msg["body"]), `"`, `\"`), msg["receiveCount"], msg["createdAt"])
			}
		}
		jsonStr += "]}"
	}

	jsonStr += fmt.Sprintf(`],
		"timestamp": "%s"
	}`, time.Now().Format(time.RFC3339))

	w.Write([]byte(jsonStr))
}
