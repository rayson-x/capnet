package cap

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Call 直连 node 的 HTTP 端点调用能力(注册表只做发现,不代理流量)。
// payload 为任务 JSON,返回 node 的响应体。
func Call(ctx context.Context, info NodeInfo, payload []byte) ([]byte, error) {
	if info.BaseURL == "" || info.Capability == "" {
		return nil, fmt.Errorf("invalid node info: %+v", info)
	}
	url := fmt.Sprintf("%s/cap/%s", info.BaseURL, info.Capability)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("call %s: status %d: %s", url, resp.StatusCode, body)
	}
	return body, nil
}
