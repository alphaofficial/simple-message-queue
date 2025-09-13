package api

import (
	"encoding/xml"
)

// JSON SQS Protocol Structures (for AWS SDK v2/v3)
type JSONSendMessageRequest struct {
	QueueUrl               string                          `json:"QueueUrl"`
	MessageBody            string                          `json:"MessageBody"`
	DelaySeconds           *int                            `json:"DelaySeconds,omitempty"`
	MessageAttributes      map[string]JSONMessageAttribute `json:"MessageAttributes,omitempty"`
	MessageDeduplicationId string                          `json:"MessageDeduplicationId,omitempty"`
	MessageGroupId         string                          `json:"MessageGroupId,omitempty"`
}

type JSONMessageAttribute struct {
	DataType    string `json:"DataType"`
	StringValue string `json:"StringValue,omitempty"`
	BinaryValue []byte `json:"BinaryValue,omitempty"`
}

type JSONSendMessageResponse struct {
	MessageId              string `json:"MessageId"`
	MD5OfMessageBody       string `json:"MD5OfMessageBody"`
	MD5OfMessageAttributes string `json:"MD5OfMessageAttributes,omitempty"`
	MessageDeduplicationId string `json:"MessageDeduplicationId,omitempty"`
	MessageGroupId         string `json:"MessageGroupId,omitempty"`
	SequenceNumber         string `json:"SequenceNumber,omitempty"`
}

type JSONReceiveMessageRequest struct {
	QueueUrl              string   `json:"QueueUrl"`
	MaxNumberOfMessages   *int     `json:"MaxNumberOfMessages,omitempty"`
	VisibilityTimeout     *int     `json:"VisibilityTimeout,omitempty"`
	WaitTimeSeconds       *int     `json:"WaitTimeSeconds,omitempty"`
	AttributeNames        []string `json:"AttributeNames,omitempty"`
	MessageAttributeNames []string `json:"MessageAttributeNames,omitempty"`
}

type JSONMessage struct {
	MessageId              string                          `json:"MessageId"`
	ReceiptHandle          string                          `json:"ReceiptHandle"`
	MD5OfBody              string                          `json:"MD5OfBody"`        // Legacy field for AWS SDK v2/consumer compatibility
	MD5OfMessageBody       string                          `json:"MD5OfMessageBody"` // Modern field for AWS SDK v3 compatibility
	MD5OfMessageAttributes string                          `json:"MD5OfMessageAttributes,omitempty"`
	Body                   string                          `json:"Body"`
	Attributes             map[string]string               `json:"Attributes,omitempty"`
	MessageAttributes      map[string]JSONMessageAttribute `json:"MessageAttributes,omitempty"`
	MessageGroupId         string                          `json:"MessageGroupId,omitempty"`
	MessageDeduplicationId string                          `json:"MessageDeduplicationId,omitempty"`
	SequenceNumber         string                          `json:"SequenceNumber,omitempty"`
}

type JSONReceiveMessageResponse struct {
	Messages []JSONMessage `json:"Messages"`
}

type JSONDeleteMessageRequest struct {
	QueueUrl      string `json:"QueueUrl"`
	ReceiptHandle string `json:"ReceiptHandle"`
}

// JSON DeleteMessageBatch types
type JSONDeleteMessageBatchRequest struct {
	QueueUrl string                        `json:"QueueUrl"`
	Entries  []JSONDeleteMessageBatchEntry `json:"Entries"`
}

type JSONDeleteMessageBatchEntry struct {
	Id            string `json:"Id"`
	ReceiptHandle string `json:"ReceiptHandle"`
}

type JSONDeleteMessageBatchResponse struct {
	Successful []JSONBatchResultEntry      `json:"Successful"`
	Failed     []JSONBatchResultErrorEntry `json:"Failed"`
}

// JSON SendMessageBatch types
type JSONSendMessageBatchRequest struct {
	QueueUrl string                      `json:"QueueUrl"`
	Entries  []JSONSendMessageBatchEntry `json:"Entries"`
}

type JSONSendMessageBatchEntry struct {
	Id                     string                          `json:"Id"`
	MessageBody            string                          `json:"MessageBody"`
	DelaySeconds           *int                            `json:"DelaySeconds,omitempty"`
	MessageAttributes      map[string]JSONMessageAttribute `json:"MessageAttributes,omitempty"`
	MessageDeduplicationId string                          `json:"MessageDeduplicationId,omitempty"`
	MessageGroupId         string                          `json:"MessageGroupId,omitempty"`
}

type JSONSendMessageBatchResponse struct {
	Successful []JSONBatchResultEntry      `json:"Successful"`
	Failed     []JSONBatchResultErrorEntry `json:"Failed"`
}

type JSONBatchResultEntry struct {
	Id                     string `json:"Id"`
	MessageId              string `json:"MessageId"`
	MD5OfMessageBody       string `json:"MD5OfMessageBody"`
	MD5OfMessageAttributes string `json:"MD5OfMessageAttributes,omitempty"`
	MessageDeduplicationId string `json:"MessageDeduplicationId,omitempty"`
	MessageGroupId         string `json:"MessageGroupId,omitempty"`
	SequenceNumber         string `json:"SequenceNumber,omitempty"`
}

type JSONBatchResultErrorEntry struct {
	Id          string `json:"Id"`
	SenderFault bool   `json:"SenderFault"`
	Code        string `json:"Code"`
	Message     string `json:"Message"`
}

type JSONErrorResponse struct {
	Type    string `json:"__type"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

// SQS Request structures
type SQSRequest struct {
	Action    string `form:"Action"`
	Version   string `form:"Version"`
	QueueName string `form:"QueueName"`
	QueueURL  string `form:"QueueUrl"`
}

type CreateQueueRequest struct {
	SQSRequest
	QueueName  string            `form:"QueueName"`
	Attributes map[string]string `form:"Attribute"`
}

type SendMessageRequest struct {
	SQSRequest
	QueueURL               string                      `form:"QueueUrl"`
	MessageBody            string                      `form:"MessageBody"`
	DelaySeconds           int                         `form:"DelaySeconds"`
	MessageAttributes      map[string]MessageAttribute `form:"MessageAttribute"`
	MessageDeduplicationId string                      `form:"MessageDeduplicationId"`
	MessageGroupId         string                      `form:"MessageGroupId"`
}

type ReceiveMessageRequest struct {
	SQSRequest
	QueueURL              string   `form:"QueueUrl"`
	MaxNumberOfMessages   int      `form:"MaxNumberOfMessages"`
	VisibilityTimeout     int      `form:"VisibilityTimeout"`
	WaitTimeSeconds       int      `form:"WaitTimeSeconds"`
	AttributeNames        []string `form:"AttributeName"`
	MessageAttributeNames []string `form:"MessageAttributeName"`
}

type DeleteMessageRequest struct {
	SQSRequest
	QueueURL      string `form:"QueueUrl"`
	ReceiptHandle string `form:"ReceiptHandle"`
}

type MessageAttribute struct {
	DataType    string `form:"DataType"`
	StringValue string `form:"StringValue"`
	BinaryValue []byte `form:"BinaryValue"`
}

// SQS Response structures
type SQSResponse struct {
	XMLName   xml.Name `xml:"Response"`
	RequestId string   `xml:"ResponseMetadata>RequestId"`
}

type CreateQueueResponse struct {
	XMLName  xml.Name `xml:"CreateQueueResponse"`
	QueueURL string   `xml:"CreateQueueResult>QueueUrl"`
	SQSResponse
}

type SendMessageResponse struct {
	XMLName                xml.Name `xml:"SendMessageResponse"`
	MessageId              string   `xml:"SendMessageResult>MessageId"`
	MD5OfBody              string   `xml:"SendMessageResult>MD5OfBody"`
	MD5OfMessageAttributes string   `xml:"SendMessageResult>MD5OfMessageAttributes"`
	// FIFO fields (only included for FIFO queues)
	MessageDeduplicationId string `xml:"SendMessageResult>MessageDeduplicationId,omitempty"`
	MessageGroupId         string `xml:"SendMessageResult>MessageGroupId,omitempty"`
	SequenceNumber         string `xml:"SendMessageResult>SequenceNumber,omitempty"`
	SQSResponse
}

type Message struct {
	MessageId              string                  `xml:"MessageId"`
	ReceiptHandle          string                  `xml:"ReceiptHandle"`
	MD5OfBody              string                  `xml:"MD5OfBody"`        // Legacy field for AWS SDK v2/consumer compatibility
	MD5OfMessageBody       string                  `xml:"MD5OfMessageBody"` // Modern field for AWS SDK v3 compatibility
	MD5OfMessageAttributes string                  `xml:"MD5OfMessageAttributes,omitempty"`
	Body                   string                  `xml:"Body"`
	Attributes             []Attribute             `xml:"Attribute"`
	MessageAttributes      []MessageAttributeValue `xml:"MessageAttribute"`

	// FIFO-specific fields (only included in response for FIFO queues)
	MessageGroupId         string `xml:"MessageGroupId,omitempty"`
	MessageDeduplicationId string `xml:"MessageDeduplicationId,omitempty"`
	SequenceNumber         string `xml:"SequenceNumber,omitempty"`
}

type Attribute struct {
	Name  string `xml:"Name"`
	Value string `xml:"Value"`
}

type MessageAttributeValue struct {
	Name        string `xml:"Name"`
	StringValue string `xml:"Value>StringValue,omitempty"`
	DataType    string `xml:"Value>DataType"`
}

type ReceiveMessageResponse struct {
	XMLName  xml.Name  `xml:"ReceiveMessageResponse"`
	Messages []Message `xml:"ReceiveMessageResult>Message"`
	SQSResponse
}

type DeleteMessageResponse struct {
	XMLName xml.Name `xml:"DeleteMessageResponse"`
	SQSResponse
}

type ListQueuesResponse struct {
	XMLName   xml.Name `xml:"ListQueuesResponse"`
	QueueURLs []string `xml:"ListQueuesResult>QueueUrl"`
	SQSResponse
}

type ErrorResponse struct {
	XMLName   xml.Name `xml:"ErrorResponse"`
	Error     Error    `xml:"Error"`
	RequestId string   `xml:"RequestId"`
}

type Error struct {
	Type    string `xml:"Type"`
	Code    string `xml:"Code"`
	Message string `xml:"Message"`
}

// Batch operation types
type BatchEntry struct {
	Id        string `xml:"Id"`
	MessageId string `xml:"MessageId"`
	MD5OfBody string `xml:"MD5OfBody"`
}

type DeleteBatchEntry struct {
	Id string `xml:"Id"`
}

type VisibilityBatchEntry struct {
	Id string `xml:"Id"`
}

type SendMessageBatchResponse struct {
	XMLName    xml.Name     `xml:"SendMessageBatchResponse"`
	Successful []BatchEntry `xml:"SendMessageBatchResult>SendMessageBatchResultEntry"`
	Failed     []BatchEntry `xml:"SendMessageBatchResult>BatchResultErrorEntry"`
	SQSResponse
}

type DeleteMessageBatchResponse struct {
	XMLName    xml.Name           `xml:"DeleteMessageBatchResponse"`
	Successful []DeleteBatchEntry `xml:"DeleteMessageBatchResult>DeleteMessageBatchResultEntry"`
	Failed     []DeleteBatchEntry `xml:"DeleteMessageBatchResult>BatchResultErrorEntry"`
	SQSResponse
}

type ChangeMessageVisibilityBatchResponse struct {
	XMLName    xml.Name               `xml:"ChangeMessageVisibilityBatchResponse"`
	Successful []VisibilityBatchEntry `xml:"ChangeMessageVisibilityBatchResult>ChangeMessageVisibilityBatchResultEntry"`
	Failed     []VisibilityBatchEntry `xml:"ChangeMessageVisibilityBatchResult>BatchResultErrorEntry"`
	SQSResponse
}

type QueueAttribute struct {
	Name  string `xml:"Name"`
	Value string `xml:"Value"`
}

type GetQueueAttributesResponse struct {
	XMLName    xml.Name         `xml:"GetQueueAttributesResponse"`
	Attributes []QueueAttribute `xml:"GetQueueAttributesResult>Attribute"`
	SQSResponse
}
