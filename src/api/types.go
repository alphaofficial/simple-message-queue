package api

import (
	"encoding/xml"
)

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
	MessageId         string                  `xml:"MessageId"`
	ReceiptHandle     string                  `xml:"ReceiptHandle"`
	MD5OfBody         string                  `xml:"MD5OfBody"`
	Body              string                  `xml:"Body"`
	Attributes        []Attribute             `xml:"Attribute"`
	MessageAttributes []MessageAttributeValue `xml:"MessageAttribute"`

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
