package backup

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"pg-admin-pro/internal/engine"
)

// S3Config holds S3 / Cloudflare R2 / MinIO off-site storage credentials.
type S3Config struct {
	Enabled        bool   `json:"enabled"`
	Endpoint       string `json:"endpoint"`
	Region         string `json:"region"`
	Bucket         string `json:"bucket"`
	Prefix         string `json:"prefix"`
	AccessKeyID    string `json:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey,omitempty"`
	ForcePathStyle bool   `json:"forcePathStyle"`
	AutoUpload     bool   `json:"autoUpload"`
}

// S3Object represents one remote backup artifact in an S3-compatible bucket.
type S3Object struct {
	Key          string `json:"key"`
	Filename     string `json:"filename"`
	SizeBytes    int64  `json:"sizeBytes"`
	SizePretty   string `json:"sizePretty"`
	LastModified string `json:"lastModified"`
	ETag         string `json:"etag"`
}

type listBucketResult struct {
	Contents []struct {
		Key          string `xml:"Key"`
		LastModified string `xml:"LastModified"`
		ETag         string `xml:"ETag"`
		Size         int64  `xml:"Size"`
	} `xml:"Contents"`
}

type s3Client struct {
	cfg  S3Config
	http *http.Client
}

func newS3Client(cfg S3Config) (*s3Client, error) {
	if strings.TrimSpace(cfg.Bucket) == "" {
		return nil, errors.New("S3 bucket is required")
	}
	if strings.TrimSpace(cfg.AccessKeyID) == "" || strings.TrimSpace(cfg.SecretAccessKey) == "" {
		return nil, errors.New("S3 accessKeyId and secretAccessKey are required")
	}
	if strings.TrimSpace(cfg.Region) == "" {
		cfg.Region = "auto"
	}
	if strings.TrimSpace(cfg.Endpoint) == "" {
		reg := cfg.Region
		if reg == "auto" {
			reg = "us-east-1"
		}
		cfg.Endpoint = fmt.Sprintf("https://s3.%s.amazonaws.com", reg)
	}
	if !strings.HasPrefix(cfg.Endpoint, "http://") && !strings.HasPrefix(cfg.Endpoint, "https://") {
		cfg.Endpoint = "https://" + cfg.Endpoint
	}
	return &s3Client{
		cfg:  cfg,
		http: &http.Client{Timeout: 30 * time.Minute},
	}, nil
}

func (c *s3Client) objectKey(filename string) string {
	clean := strings.TrimLeft(filename, "/")
	pfx := strings.Trim(c.cfg.Prefix, "/")
	if pfx == "" {
		return clean
	}
	return pfx + "/" + clean
}

func (c *s3Client) buildURL(key string, query url.Values) (*url.URL, string, error) {
	u, err := url.Parse(strings.TrimRight(c.cfg.Endpoint, "/"))
	if err != nil {
		return nil, "", fmt.Errorf("invalid S3 endpoint: %w", err)
	}
	cleanKey := strings.TrimLeft(key, "/")
	basePath := strings.TrimRight(u.Path, "/")
	if c.cfg.ForcePathStyle || key == "" || !strings.Contains(u.Host, "amazonaws.com") {
		rawPath := basePath + "/" + c.cfg.Bucket
		rawEscaped := encodeS3Path(strings.TrimLeft(basePath, "/"))
		if rawEscaped != "" {
			rawEscaped = "/" + rawEscaped
		}
		rawEscaped += "/" + awsEscape(c.cfg.Bucket)
		if cleanKey != "" {
			rawPath += "/" + cleanKey
			rawEscaped += "/" + encodeS3Path(cleanKey)
		}
		u.Path = rawPath
		u.RawPath = rawEscaped
	} else {
		u.Host = c.cfg.Bucket + "." + u.Host
		rawPath := basePath + "/" + cleanKey
		rawEscaped := encodeS3Path(strings.TrimLeft(basePath, "/"))
		if rawEscaped != "" {
			rawEscaped = "/" + rawEscaped
		}
		rawEscaped += "/" + encodeS3Path(cleanKey)
		u.Path = rawPath
		u.RawPath = rawEscaped
	}
	if u.Path == "" {
		u.Path = "/"
		u.RawPath = "/"
	}
	canonURI := u.EscapedPath()
	if canonURI == "" {
		canonURI = "/"
	}
	if query != nil {
		u.RawQuery = encodeCanonicalQuery(query)
	}
	return u, canonURI, nil
}

func encodeS3Path(p string) string {
	if p == "" {
		return ""
	}
	segs := strings.Split(p, "/")
	for i, s := range segs {
		segs[i] = awsEscape(s)
	}
	return strings.Join(segs, "/")
}

func encodeCanonicalQuery(q url.Values) string {
	if len(q) == 0 {
		return ""
	}
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		vals := append([]string(nil), q[k]...)
		sort.Strings(vals)
		for _, v := range vals {
			parts = append(parts, awsEscape(k)+"="+awsEscape(v))
		}
	}
	return strings.Join(parts, "&")
}

func awsEscape(s string) string {
	escaped := url.QueryEscape(s)
	escaped = strings.ReplaceAll(escaped, "+", "%20")
	escaped = strings.ReplaceAll(escaped, "*", "%2A")
	escaped = strings.ReplaceAll(escaped, "%7E", "~")
	return escaped
}

func (c *s3Client) doSigned(ctx context.Context, method, key string, query url.Values, body io.Reader, contentLen int64, payloadHash string) (*http.Response, error) {
	u, canonURI, err := c.buildURL(key, query)
	if err != nil {
		return nil, err
	}
	if payloadHash == "" {
		h := sha256.Sum256(nil)
		payloadHash = hex.EncodeToString(h[:])
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, err
	}
	if contentLen > 0 {
		req.ContentLength = contentLen
	}

	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")

	req.Header.Set("Host", u.Host)
	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", payloadHash)

	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonHeaders := fmt.Sprintf("host:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n", u.Host, payloadHash, amzDate)
	canonQuery := u.RawQuery

	canonReq := strings.Join([]string{
		method,
		canonURI,
		canonQuery,
		canonHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	reqHash := sha256.Sum256([]byte(canonReq))
	scope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, c.cfg.Region)
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		hex.EncodeToString(reqHash[:]),
	}, "\n")

	kDate := hmacSHA256([]byte("AWS4"+c.cfg.SecretAccessKey), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(c.cfg.Region))
	kService := hmacSHA256(kRegion, []byte("s3"))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))
	sig := hex.EncodeToString(hmacSHA256(kSigning, []byte(stringToSign)))

	authHeader := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		c.cfg.AccessKeyID, scope, signedHeaders, sig)
	req.Header.Set("Authorization", authHeader)

	return c.http.Do(req)
}

func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

func readS3Error(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		msg = resp.Status
	}
	return fmt.Errorf("S3 HTTP %d: %s", resp.StatusCode, msg)
}

func (c *s3Client) listObjects(ctx context.Context, maxKeys string) ([]S3Object, error) {
	q := url.Values{}
	q.Set("list-type", "2")
	if pfx := strings.Trim(c.cfg.Prefix, "/"); pfx != "" {
		q.Set("prefix", pfx+"/")
	}
	if maxKeys != "" {
		q.Set("max-keys", maxKeys)
	}
	resp, err := c.doSigned(ctx, http.MethodGet, "", q, nil, 0, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, readS3Error(resp)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var parsed listBucketResult
	if err := xml.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("parse S3 XML response: %w", err)
	}

	out := make([]S3Object, 0, len(parsed.Contents))
	for _, obj := range parsed.Contents {
		if strings.HasSuffix(obj.Key, "/") {
			continue
		}
		out = append(out, S3Object{
			Key:          obj.Key,
			Filename:     path.Base(obj.Key),
			SizeBytes:    obj.Size,
			SizePretty:   engine.HumanBytes(obj.Size),
			LastModified: obj.LastModified,
			ETag:         strings.Trim(obj.ETag, `"`),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].LastModified > out[j].LastModified
	})
	return out, nil
}

func (c *s3Client) uploadFile(ctx context.Context, localPath, key string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return err
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	payloadHash := hex.EncodeToString(h.Sum(nil))

	resp, err := c.doSigned(ctx, http.MethodPut, key, nil, f, fi.Size(), payloadHash)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return readS3Error(resp)
	}
	return nil
}

func (c *s3Client) downloadFile(ctx context.Context, key, destPath string) error {
	resp, err := c.doSigned(ctx, http.MethodGet, key, nil, nil, 0, "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return readS3Error(resp)
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o750); err != nil {
		return err
	}
	tmp := destPath + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, resp.Body)
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if err := os.Rename(tmp, destPath); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func (c *s3Client) deleteObject(ctx context.Context, key string) error {
	resp, err := c.doSigned(ctx, http.MethodDelete, key, nil, bytes.NewReader(nil), 0, "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusNotFound {
		return readS3Error(resp)
	}
	return nil
}
