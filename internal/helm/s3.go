package helm

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type s3Getter struct {
	mu      sync.Mutex
	clients map[string]*s3.Client
}

func (s *s3Getter) getClient(region string) (*s3.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.clients == nil {
		s.clients = make(map[string]*s3.Client)
	}
	if c, ok := s.clients[region]; ok {
		return c, nil
	}
	opts := []func(*config.LoadOptions) error{}
	if region != "" {
		opts = append(opts, config.WithRegion(region))
	}
	cfg, err := config.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("unable to load AWS config: %w", err)
	}
	client := s3.NewFromConfig(cfg)
	s.clients[region] = client
	return client, nil
}

func (s *s3Getter) GetWithRegion(s3Url, region string) (*bytes.Buffer, error) {
	u, err := url.Parse(s3Url)
	if err != nil {
		return nil, fmt.Errorf("invalid s3 URL format: %s", s3Url)
	}
	bucket := u.Host
	key := u.Path
	if key == "" {
		key = "/"
	}
	if key[0] == '/' {
		key = key[1:]
	}
	client, err := s.getClient(region)
	if err != nil {
		return nil, err
	}
	resp, err := client.GetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("unable to get s3 object: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	var ret bytes.Buffer
	if _, err = io.Copy(&ret, resp.Body); err != nil {
		return nil, fmt.Errorf("unable to read s3 object: %w", err)
	}
	return &ret, nil
}
