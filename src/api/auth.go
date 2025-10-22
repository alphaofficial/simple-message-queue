package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"simple-message-queue/src/storage"
)

// AWS-style access key format: AKIA + 16 chars
func GenerateAccessKeyID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return "AKIA" + strings.ToUpper(hex.EncodeToString(b))
}

// AWS-style secret access key: 40 character base64-like string
func GenerateSecretAccessKey() string {
	b := make([]byte, 30)
	rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

// AWS Signature Version 4 style authentication
type AWSAuth struct {
	AccessKeyID     string
	SecretAccessKey string
	Region          string
	Service         string
}

// Extract and validate AWS signature from request
func ExtractAWSSignature(r *http.Request) (*AWSSignatureData, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil, fmt.Errorf("missing Authorization header")
	}

	// Parse AWS4-HMAC-SHA256 Credential=...,SignedHeaders=...,Signature=...
	if !strings.HasPrefix(authHeader, "AWS4-HMAC-SHA256 ") {
		return nil, fmt.Errorf("unsupported authorization type")
	}

	parts := strings.Split(strings.TrimPrefix(authHeader, "AWS4-HMAC-SHA256 "), ",")
	sig := &AWSSignatureData{}

	for _, part := range parts {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}

		switch kv[0] {
		case "Credential":
			credParts := strings.Split(kv[1], "/")
			if len(credParts) > 0 {
				sig.AccessKeyID = credParts[0]
			}
			if len(credParts) > 1 {
				sig.Date = credParts[1]
			}
			if len(credParts) > 2 {
				sig.Region = credParts[2]
			}
			if len(credParts) > 3 {
				sig.Service = credParts[3]
			}
		case "SignedHeaders":
			sig.SignedHeaders = kv[1]
		case "Signature":
			sig.Signature = kv[1]
		}
	}

	// Also extract timestamp
	sig.Timestamp = r.Header.Get("X-Amz-Date")
	if sig.Timestamp == "" {
		sig.Timestamp = r.Header.Get("Date")
	}

	return sig, nil
}

type AWSSignatureData struct {
	AccessKeyID   string
	Date          string
	Region        string
	Service       string
	SignedHeaders string
	Signature     string
	Timestamp     string
}

// Verify AWS signature
func VerifyAWSSignature(r *http.Request, secretKey string, sigData *AWSSignatureData) error {
	// Get the request body for signature calculation
	var payload string
	if r.Body != nil {
		// This is simplified - in practice you'd need to read and restore the body
		payload = ""
	}

	// Build canonical request
	canonicalRequest := buildCanonicalRequest(r, sigData.SignedHeaders, payload)

	// Build string to sign
	stringToSign := buildStringToSign(sigData.Timestamp, sigData.Date, sigData.Region, sigData.Service, canonicalRequest)

	// Calculate signature
	expectedSignature := calculateSignature(secretKey, sigData.Date, sigData.Region, sigData.Service, stringToSign)

	if sigData.Signature != expectedSignature {
		return fmt.Errorf("signature mismatch")
	}

	return nil
}

func buildCanonicalRequest(r *http.Request, signedHeaders, payload string) string {
	// HTTP method
	method := r.Method

	// Canonical URI
	uri := r.URL.Path
	if uri == "" {
		uri = "/"
	}

	// Canonical query string
	query := ""
	if r.URL.RawQuery != "" {
		params := r.URL.Query()
		var keys []string
		for k := range params {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		var parts []string
		for _, k := range keys {
			for _, v := range params[k] {
				parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(v))
			}
		}
		query = strings.Join(parts, "&")
	}

	// Canonical headers
	headerNames := strings.Split(signedHeaders, ";")
	var canonicalHeaders []string
	for _, name := range headerNames {
		value := r.Header.Get(name)
		canonicalHeaders = append(canonicalHeaders, strings.ToLower(name)+":"+strings.TrimSpace(value))
	}
	headers := strings.Join(canonicalHeaders, "\n") + "\n"

	// Payload hash
	payloadHash := sha256.Sum256([]byte(payload))
	payloadHashHex := hex.EncodeToString(payloadHash[:])

	return strings.Join([]string{
		method,
		uri,
		query,
		headers,
		signedHeaders,
		payloadHashHex,
	}, "\n")
}

func buildStringToSign(timestamp, date, region, service, canonicalRequest string) string {
	algorithm := "AWS4-HMAC-SHA256"
	credentialScope := strings.Join([]string{date, region, service, "aws4_request"}, "/")

	hash := sha256.Sum256([]byte(canonicalRequest))
	hashedCanonicalRequest := hex.EncodeToString(hash[:])

	return strings.Join([]string{
		algorithm,
		timestamp,
		credentialScope,
		hashedCanonicalRequest,
	}, "\n")
}

func calculateSignature(secretKey, date, region, service, stringToSign string) string {
	kSecret := []byte("AWS4" + secretKey)
	kDate := hmacSHA256(kSecret, date)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, service)
	kSigning := hmacSHA256(kService, "aws4_request")
	signature := hmacSHA256(kSigning, stringToSign)

	return hex.EncodeToString(signature)
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

// Middleware for authenticating API requests
func (h *SMQHandler) authenticateRequest(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Skip authentication for dashboard and login endpoints
		if strings.HasPrefix(r.URL.Path, "/api/") && !strings.HasPrefix(r.URL.Path, "/api/auth/") {
			// Dashboard API endpoints use session auth
			next(w, r)
			return
		}

		if r.URL.Path == "/" || r.URL.Path == "/login" || r.URL.Path == "/logout" || r.URL.Path == "/health" {
			next(w, r)
			return
		}

		// Extract AWS signature
		sigData, err := ExtractAWSSignature(r)
		if err != nil {
			h.writeErrorResponse(w, "InvalidAccessKeyId", "Invalid access key ID", http.StatusForbidden)
			return
		}

		// Get access key from storage
		accessKey, err := h.storage.GetAccessKey(r.Context(), sigData.AccessKeyID)
		if err != nil {
			h.writeErrorResponse(w, "InternalError", "Failed to validate access key", http.StatusInternalServerError)
			return
		}

		if accessKey == nil || !accessKey.Active {
			h.writeErrorResponse(w, "InvalidAccessKeyId", "The AWS access key ID you provided does not exist in our records", http.StatusForbidden)
			return
		}

		// Verify signature
		if err := VerifyAWSSignature(r, accessKey.SecretAccessKey, sigData); err != nil {
			h.writeErrorResponse(w, "SignatureDoesNotMatch", "The request signature we calculated does not match the signature you provided", http.StatusForbidden)
			return
		}

		// Update last used timestamp
		h.storage.UpdateAccessKeyUsage(r.Context(), accessKey.AccessKeyID)

		next(w, r)
	}
}

// Access key management endpoints
func (h *SMQHandler) handleCreateAccessKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, `{"error": "Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if request.Name == "" {
		http.Error(w, `{"error": "Name is required"}`, http.StatusBadRequest)
		return
	}

	// Generate new access key
	accessKey := &storage.AccessKey{
		AccessKeyID:     GenerateAccessKeyID(),
		SecretAccessKey: GenerateSecretAccessKey(),
		Name:            request.Name,
		Description:     request.Description,
		Active:          true,
		CreatedAt:       time.Now(),
	}

	if err := h.storage.CreateAccessKey(r.Context(), accessKey); err != nil {
		http.Error(w, `{"error": "Failed to create access key"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"accessKeyId":     accessKey.AccessKeyID,
		"secretAccessKey": accessKey.SecretAccessKey,
		"name":            accessKey.Name,
		"description":     accessKey.Description,
		"active":          accessKey.Active,
		"createdAt":       accessKey.CreatedAt,
	})
}

func (h *SMQHandler) handleListAccessKeys(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	accessKeys, err := h.storage.ListAccessKeys(r.Context())
	if err != nil {
		http.Error(w, `{"error": "Failed to list access keys"}`, http.StatusInternalServerError)
		return
	}

	// Don't return secret keys in list
	var response []map[string]interface{}
	for _, key := range accessKeys {
		keyData := map[string]interface{}{
			"accessKeyId": key.AccessKeyID,
			"name":        key.Name,
			"description": key.Description,
			"active":      key.Active,
			"createdAt":   key.CreatedAt,
		}
		if key.LastUsedAt != nil {
			keyData["lastUsedAt"] = key.LastUsedAt
		}
		response = append(response, keyData)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"accessKeys": response,
	})
}

func (h *SMQHandler) handleDeactivateAccessKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request struct {
		AccessKeyID string `json:"accessKeyId"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, `{"error": "Invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if request.AccessKeyID == "" {
		http.Error(w, `{"error": "AccessKeyID is required"}`, http.StatusBadRequest)
		return
	}

	if err := h.storage.DeactivateAccessKey(r.Context(), request.AccessKeyID); err != nil {
		http.Error(w, `{"error": "Failed to deactivate access key"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	})
}

func (h *SMQHandler) handleDeleteAccessKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract access key ID from URL path
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 5 {
		http.Error(w, `{"error": "Invalid path"}`, http.StatusBadRequest)
		return
	}
	accessKeyID := pathParts[4]

	if err := h.storage.DeleteAccessKey(r.Context(), accessKeyID); err != nil {
		http.Error(w, `{"error": "Failed to delete access key"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	})
}
