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
	SQSResponse
}

type Message struct {
	MessageId         string                      `xml:"MessageId"`
	ReceiptHandle     string                      `xml:"ReceiptHandle"`
	MD5OfBody         string                      `xml:"MD5OfBody"`
	Body              string                      `xml:"Body"`
	Attributes        map[string]string           `xml:"Attribute"`
	MessageAttributes map[string]MessageAttribute `xml:"MessageAttribute"`
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
