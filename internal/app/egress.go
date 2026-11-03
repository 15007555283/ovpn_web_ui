package app

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type EgressStatus struct {
	IP        string    `json:"ip"`
	Message   string    `json:"message"`
	CheckedAt time.Time `json:"checked_at"`
}

func queryEgressIP(ctx context.Context, client *http.Client) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.ipify.org", nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errors.New("出口 IP 查询失败")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 65))
	if err != nil {
		return "", err
	}
	ip, err := netip.ParseAddr(strings.TrimSpace(string(data)))
	if err != nil || !ip.Is4() || !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return "", errors.New("出口 IP 响应无效")
	}
	return ip.String(), nil
}

func (a *App) egressIP(c *gin.Context) {
	if a.c.Fake {
		success(c, EgressStatus{Message: "演示模式不探测公网出口"})
		return
	}
	// API 串行锁保护缓存；失败也短暂缓存，避免不断访问外部服务。
	ttl := 5 * time.Minute
	if a.egressStatus.IP == "" {
		ttl = time.Minute
	}
	if !a.egressStatus.CheckedAt.IsZero() && time.Since(a.egressStatus.CheckedAt) < ttl {
		success(c, a.egressStatus)
		return
	}
	// 不使用环境代理，否则得到的是代理出口；当前 VPN 仅支持 IPv4。
	transport := &http.Transport{DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "tcp4", addr)
	}, TLSHandshakeTimeout: 3 * time.Second}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	ip, err := queryEgressIP(c.Request.Context(), client)
	status := EgressStatus{IP: ip, CheckedAt: time.Now()}
	if err != nil {
		status.Message = "暂时无法获取出口 IP"
	}
	a.egressStatus = status
	success(c, status)
}
