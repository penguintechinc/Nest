// Package sigv4 implements AWS Signature Version 4 request signing.
//
// It is a leaf package by design: it imports nothing from this repository, so
// both pkg/provider (S3-compatible object storage) and pkg/cloudprovider
// (AWS management-plane APIs) can depend on it without an import cycle —
// pkg/cloudprovider already imports pkg/provider, which rules out putting the
// signer in either of them.
//
// The signature scheme is used unmodified by every S3-compatible provider
// (DigitalOcean Spaces, Vultr Object Storage, Linode Object Storage, MinIO,
// Ceph RGW), which differ only in endpoint host and region string.
package sigv4

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// timeNow is a seam so tests can pin the signing timestamp. AWS SigV4 folds
// the date into both the credential scope and the derived signing key, so a
// signature is only reproducible against a fixed clock.
var timeNow = time.Now

// Sign signs req in place with AWS Signature Version 4, setting the
// X-Amz-Date, x-amz-content-sha256, optional X-Amz-Security-Token, and
// Authorization headers.
//
// body must be the exact bytes that will be sent; pass nil for an empty body.
// service is the AWS service name used in the credential scope ("s3", "rds",
// "ec2", …). For S3-compatible providers, service is always "s3" and region is
// whatever the provider expects (for example "nyc3" for DigitalOcean Spaces).
func Sign(req *http.Request, service, region, accessKey, secretKey, sessionToken string, body []byte) error {
	if req == nil {
		return fmt.Errorf("sigv4: request is nil")
	}
	if accessKey == "" || secretKey == "" {
		return fmt.Errorf("sigv4: accessKey and secretKey are required")
	}

	now := timeNow().UTC()
	amzDate := now.Format("20060102T150405Z")
	datestamp := now.Format("20060102")

	if body == nil {
		body = []byte{}
	}

	payloadHash := sha256sum(body)
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("x-amz-content-sha256", payloadHash)

	if sessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", sessionToken)
	}

	canonicalRequest := buildCanonicalRequest(req, payloadHash)
	stringToSign := buildStringToSign(canonicalRequest, amzDate, datestamp, region, service)
	signature := calculateSignature(stringToSign, secretKey, datestamp, region, service)

	credentialScope := fmt.Sprintf("%s/%s/%s/aws4_request", datestamp, region, service)
	signedHeaders := getSignedHeaders(req)
	authHeader := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKey, credentialScope, signedHeaders, signature)

	req.Header.Set("Authorization", authHeader)

	return nil
}

// requestHost returns the host to sign. Go keeps the Host outside of
// req.Header — http.NewRequest populates req.Host (or leaves it empty and
// relies on req.URL.Host) and the transport writes it at send time. Ranging
// over req.Header therefore never yields a "host" key, so it has to be pulled
// out explicitly. AWS rejects any signature whose SignedHeaders omits host.
func requestHost(req *http.Request) string {
	if req.Host != "" {
		return req.Host
	}
	if req.URL != nil {
		return req.URL.Host
	}
	return ""
}

func buildCanonicalRequest(req *http.Request, payloadHash string) string {
	method := req.Method
	canonicalURI := getCanonicalURI(req.URL.Path)
	canonicalQueryString := getCanonicalQueryString(req.URL)
	canonicalHeaders := getCanonicalHeaders(req)
	signedHeaders := getSignedHeaders(req)

	return fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
		method,
		canonicalURI,
		canonicalQueryString,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	)
}

func getCanonicalURI(path string) string {
	if path == "" {
		return "/"
	}
	return path
}

func getCanonicalQueryString(u *url.URL) string {
	if u.RawQuery == "" {
		return ""
	}

	params := u.Query()
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var buf bytes.Buffer
	for i, k := range keys {
		if i > 0 {
			buf.WriteString("&")
		}
		buf.WriteString(url.QueryEscape(k))
		buf.WriteString("=")
		buf.WriteString(url.QueryEscape(params.Get(k)))
	}
	return buf.String()
}

// signableHeaders collects the lowercase header names and values that take
// part in the signature: host, plus every x-amz-* header.
func signableHeaders(req *http.Request) map[string]string {
	headers := make(map[string]string)

	if h := requestHost(req); h != "" {
		headers["host"] = h
	}

	for k, vv := range req.Header {
		lowerK := strings.ToLower(k)
		if len(vv) == 0 {
			continue
		}
		if lowerK == "host" || strings.HasPrefix(lowerK, "x-amz-") {
			headers[lowerK] = strings.TrimSpace(vv[0])
		}
	}
	return headers
}

func getCanonicalHeaders(req *http.Request) string {
	headers := signableHeaders(req)

	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var buf bytes.Buffer
	for _, k := range keys {
		buf.WriteString(k)
		buf.WriteString(":")
		buf.WriteString(headers[k])
		buf.WriteString("\n")
	}
	return buf.String()
}

func getSignedHeaders(req *http.Request) string {
	headers := signableHeaders(req)

	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	return strings.Join(keys, ";")
}

func buildStringToSign(canonicalRequest, amzDate, datestamp, region, service string) string {
	canonicalRequestHash := sha256sum([]byte(canonicalRequest))
	credentialScope := fmt.Sprintf("%s/%s/%s/aws4_request", datestamp, region, service)

	return fmt.Sprintf("AWS4-HMAC-SHA256\n%s\n%s\n%s",
		amzDate,
		credentialScope,
		canonicalRequestHash,
	)
}

func calculateSignature(stringToSign, secretKey, datestamp, region, service string) string {
	kDate := hmacsha256([]byte("AWS4"+secretKey), []byte(datestamp))
	kRegion := hmacsha256(kDate, []byte(region))
	kService := hmacsha256(kRegion, []byte(service))
	kSigning := hmacsha256(kService, []byte("aws4_request"))

	return hex.EncodeToString(hmacsha256(kSigning, []byte(stringToSign)))
}

func sha256sum(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func hmacsha256(key, msg []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(msg)
	return h.Sum(nil)
}
