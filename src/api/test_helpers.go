package api

import (
	"fmt"
	"net/http"
)

// HandleSQSRequest is a test helper function that routes SQS protocol requests to the appropriate handler
// This function is only used for testing and provides access to the unexported handler methods
func (h *SMQHandler) HandleSQSRequest(w http.ResponseWriter, r *http.Request) {
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
