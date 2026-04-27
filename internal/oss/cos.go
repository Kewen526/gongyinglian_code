package oss

import (
	"fmt"
	"io"
	"path"
	"supply-chain/internal/config"
	"time"

	alioss "github.com/aliyun/aliyun-oss-go-sdk/oss"
)

var bucket *alioss.Bucket

func InitOSS(cfg *config.OSSConfig) {
	client, err := alioss.New(cfg.Endpoint, cfg.AccessKeyID, cfg.AccessKeySecret)
	if err != nil {
		panic(fmt.Sprintf("failed to init Alibaba Cloud OSS client: %v", err))
	}
	b, err := client.Bucket(cfg.Bucket)
	if err != nil {
		panic(fmt.Sprintf("failed to get OSS bucket: %v", err))
	}
	bucket = b
}

// Upload uploads a file to OSS and returns the public URL.
// folder: e.g. "product/images", "product/videos"
func Upload(folder string, filename string, reader io.Reader) (string, error) {
	cfg := config.GlobalConfig.OSS

	ext := path.Ext(filename)
	key := fmt.Sprintf("%s/%d%s", folder, time.Now().UnixNano(), ext)

	if err := bucket.PutObject(key, reader); err != nil {
		return "", fmt.Errorf("OSS upload failed: %w", err)
	}

	fileURL := fmt.Sprintf("https://%s.%s/%s", cfg.Bucket, cfg.Endpoint, key)
	return fileURL, nil
}
